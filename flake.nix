{
  description = "CommonMark package for Go";

  inputs = {
    nixpkgs.url = "nixpkgs";
    flake-utils.url = "flake-utils";
  };

  outputs = { self, flake-utils, ... }@inputs:
    flake-utils.lib.eachDefaultSystem (system:
    let
      pkgs = import inputs.nixpkgs { inherit system; };
    in
    {
      devShells.default = pkgs.mkShell {
        packages = [
          inputs.nixpkgs.legacyPackages.${system}.go-tools
          inputs.nixpkgs.legacyPackages.${system}.go_1_20
        ];
      };
    });
}
