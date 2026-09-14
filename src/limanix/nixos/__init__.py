"""Packaged NixOS modules and per-machine configuration bundles."""

from limanix.nixos.bundle import builtin_modules, prepare_bundle

__all__ = ["builtin_modules", "prepare_bundle"]
