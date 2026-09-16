{ inputs, lib, modulesPath, ... }:
{
  imports = [
    inputs.nixos-lima.nixosModules.lima
    (modulesPath + "/profiles/qemu-guest.nix")
  ];

  services.lima.enable = true;
  users.mutableUsers = true;
  services.openssh.enable = true;
  nix.settings.experimental-features = [ "nix-command" "flakes" ];

  # Partition layout of the pinned nixos-lima v0.2.1 base images.
  # Grow the root partition before autoResize expands its filesystem on boot.
  boot.growPartition = true;
  boot.loader.grub = {
    device = "nodev";
    efiSupport = true;
    efiInstallAsRemovable = true;
  };
  fileSystems."/boot" = {
    device = lib.mkForce "/dev/vda1";
    fsType = "vfat";
  };
  fileSystems."/" = {
    device = "/dev/disk/by-label/nixos";
    autoResize = true;
    fsType = "ext4";
    options = [ "noatime" "nodiratime" "discard" ];
  };

  system.stateVersion = "26.05";
}
