{
  description = "parsh — interactive shell for AWS SSM Parameter Store";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in {
      packages = forAll (pkgs: {
        default = pkgs.buildGoModule {
          pname = "parsh";
          version = "v1.0.0";
          src = ./.;
          vendorHash = "sha256-ojWjqz0RSGPlo7WO2FiHLArvN+B7u1b+NigYejGjd6o=";
          ldflags = [ "-s" "-w" "-X main.Version=v1.0.0" ];
          meta = with pkgs.lib; {
            description = "Interactive shell for AWS SSM Parameter Store (forked from ssmsh)";
            homepage = "https://github.com/torreirow/parsh";
            license = licenses.mit;
            mainProgram = "parsh";
          };
        };
      });

      apps = forAll (pkgs: {
        default = {
          type = "app";
          program = "${self.packages.${pkgs.system}.default}/bin/parsh";
        };
      });
    };
}
