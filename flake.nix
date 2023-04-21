{
  description = "CommonMark package for Go";

  inputs = {
    nixpkgs.url = "nixpkgs";
    flake-utils.url = "flake-utils";
  };

  outputs = { self, flake-utils, ... }@inputs:
    flake-utils.lib.eachDefaultSystem (system:
    let
      pkgs = import inputs.nixpkgs {
        inherit system;
        overlays = [ self.overlays.cmark ];
      };
    in
    {
      packages.commonmark-js = (pkgs.callPackage ./nix/commonmark.js {})."commonmark-0.30.0";

      devShells.default = pkgs.mkShell {
        packages = [
          pkgs.cmark
          pkgs.go-tools
          pkgs.go_1_20
          pkgs.node2nix
          self.packages.${system}.commonmark-js
        ];
      };
    }) // {
      overlays.cmark = final: prev: {
        cmark = prev.cmark.overrideAttrs (oldAttrs: let version = "0.30.3"; in {
          inherit version;

          src = prev.fetchFromGitHub {
            owner = "commonmark";
            repo = oldAttrs.pname;
            rev = version;
            hash = "sha256-/7TzaZYP8lndkfRPgCpBbazUBytVLXxqWHYktIsGox0=";
          };
        });
      };
    };
}
