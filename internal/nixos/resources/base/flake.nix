{
  description = "Limanix guest configuration";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    nixos-lima = {
      url = "github:nixos-lima/nixos-lima/v0.2.1";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = inputs@{ nixpkgs, ... }:
    let
      runtime = builtins.fromJSON (builtins.readFile ./runtime.json);
      systems = {
        arm64 = "aarch64-linux";
        amd64 = "x86_64-linux";
      };
    in {
      nixosConfigurations.runtime = nixpkgs.lib.nixosSystem {
        specialArgs = { inherit inputs runtime; };
        modules = [
          { nixpkgs.hostPlatform = systems.${runtime.arch}; }
          ./platform.nix
          ./runtime.nix
        ]
          ++ builtins.map (path: ./. + "/${path}") runtime.modules;
      };
    };
}
