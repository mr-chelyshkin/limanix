{ pkgs, ... }:
{
  environment.systemPackages = with pkgs; [
    curl
    jq
    ripgrep
  ];
}
