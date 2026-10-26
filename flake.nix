{
  description = "parsh — interactive shell for AWS SSM Parameter Store";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      # Single source of truth for the version, shared with release.sh and GoReleaser
      version = nixpkgs.lib.removeSuffix "\n" (builtins.readFile ./VERSION);
    in {
      packages = forAll (pkgs: {
        default = pkgs.buildGoModule {
          pname = "parsh";
          inherit version;
          src = ./.;
          vendorHash = "sha256-ojWjqz0RSGPlo7WO2FiHLArvN+B7u1b+NigYejGjd6o=";
          ldflags = [ "-s" "-w" "-X main.Version=${version}" ];
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
          program = "${self.packages.${pkgs.stdenv.hostPlatform.system}.default}/bin/parsh";
        };
      });
    };
}
