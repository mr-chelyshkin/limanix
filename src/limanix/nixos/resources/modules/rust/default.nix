{ pkgs, ... }:
{
  environment.systemPackages = with pkgs; [
    rustc
    cargo
    rustfmt
    clippy
    rust-analyzer
    gcc
    pkg-config
  ];
}
