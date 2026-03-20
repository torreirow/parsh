{
  description = "ssmsh — interactive shell for AWS SSM Parameter Store";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in {
      packages = forAll (pkgs: {
        default = pkgs.buildGoModule {
          pname = "ssmsh";
          version = "v1.5.0";
          src = ./.;
          vendorHash = "sha256-+7duWRe/haBOZbe18sr2qwg419ieEZwYDb0L3IPLA4A=";
          ldflags = [ "-s" "-w" "-X main.Version=v1.5.0" ];
          meta = with pkgs.lib; {
            description = "Interactive shell for AWS SSM Parameter Store";
            homepage = "https://github.com/torreirow/ssmsh";
            license = licenses.mit;
            mainProgram = "ssmsh";
          };
        };
      });

      apps = forAll (pkgs: {
        default = {
          type = "app";
          program = "${self.packages.${pkgs.system}.default}/bin/ssmsh";
        };
      });
    };
}
