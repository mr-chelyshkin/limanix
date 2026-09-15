{ lib, pkgs, runtime, ... }:
let
  environmentDropIn = ''
    [Service]
    EnvironmentFile=-/etc/limanix/environment
  '';
in
{
  networking.hostName = runtime.name;
  networking.firewall.allowedTCPPorts = runtime.ports.tcp;
  networking.firewall.allowedUDPPorts = runtime.ports.udp;

  # Lima registers its fstab mounts during lima-init, after local-fs.target.
  # Its nofail mounts do not order themselves before that target. Finish them
  # before ordinary services and lingering user managers can access their files.
  # Limanix does not expose Lima provision scripts: upstream user provisioning
  # waits for a user manager and cannot run before basic.target.
  systemd.services.lima-init = {
    unitConfig.DefaultDependencies = false;
    requires = [ "sysinit.target" ];
    after = lib.mkForce [ "sysinit.target" ];
    before = [ "basic.target" "shutdown.target" ];
    conflicts = [ "shutdown.target" ];
    requiredBy = [ "basic.target" ];
    postStart = ''
      set -o pipefail
      directory="$(${pkgs.coreutils}/bin/mktemp -d /run/limanix-mounts.XXXXXX)"
      trap '${pkgs.coreutils}/bin/rm -rf -- "$directory"' EXIT
      # Read the current Lima block: an update first boots the previous NixOS
      # generation with the newly edited Lima mount configuration.
      ${pkgs.gawk}/bin/awk '
        /^#LIMA-START$/ { lima = 1; next }
        /^#LIMA-END$/ { lima = 0 }
        lima
      ' /etc/fstab > "$directory/fstab"
      ${pkgs.util-linux}/bin/findmnt --fstab --tab-file "$directory/fstab" \
        --json --output TARGET \
        | ${pkgs.jq}/bin/jq -j '.filesystems[].target + "\u0000"' \
        > "$directory/targets"
      while IFS= read -r -d "" target; do
        unit="$(${pkgs.systemd}/bin/systemd-escape --path --suffix=mount "$target")"
        ${pkgs.systemd}/bin/systemctl start -- "$unit"
      done < "$directory/targets"
    '';
  };

  # macOS primary group IDs can collide with NixOS system groups. Keep the
  # host UID for mount ownership and let NixOS allocate a persistent guest GID.
  users.groups.${runtime.user.name} = { };
  users.users.${runtime.user.name} = {
    # NixOS reserves the normal-user classification for UIDs >= 1000. macOS
    # commonly assigns lower UIDs; these accounts still get a login shell.
    isNormalUser = runtime.user.uid >= 1000;
    isSystemUser = runtime.user.uid < 1000;
    uid = runtime.user.uid;
    group = runtime.user.name;
    home = runtime.user.home;
    shell = pkgs.bashInteractive;
    createHome = false;
    linger = true;
  };
  security.sudo.wheelNeedsPassword = true;
  security.sudo.extraRules = [
    {
      users = [ "limanix-admin" ]
        ++ lib.optional runtime.user.sudo runtime.user.name;
      commands = [{ command = "ALL"; options = [ "NOPASSWD" ]; }];
    }
  ];

  # Values stay in runtime files, outside flake sources and /nix/store.
  environment.extraInit = ''
    # sudo changes users inside the management SSH session; pam_systemd does
    # not export the target user's runtime directory in this situation.
    if [ "$(${pkgs.coreutils}/bin/id -u)" = "${toString runtime.user.uid}" ]; then
      export XDG_RUNTIME_DIR="/run/user/${toString runtime.user.uid}"
      export DBUS_SESSION_BUS_ADDRESS="unix:path=$XDG_RUNTIME_DIR/bus"
    fi
    if [ -r /etc/limanix/environment.sh ]; then
      . /etc/limanix/environment.sh
    fi
  '';
  systemd.packages = builtins.map (scope:
    pkgs.writeTextDir
      "lib/systemd/${scope}/service.d/10-limanix-environment.conf"
      environmentDropIn
  ) [ "system" "user" ];
}
