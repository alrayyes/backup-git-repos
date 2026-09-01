{
  description = "Mirror every repository on a self-hosted GitLab or Forgejo instance, or a GitHub.com account, to a local, restorable backup";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "backup-git-repos";
          version = self.shortRev or self.dirtyShortRev or "dev";

          src = ./.;
          subPackages = [ "cmd/backup-git-repos" ];

          vendorHash = "sha256-q1dnpDN2/9IEy/Z1X9lz6aPepyvxhBK6r2gy/QQbxdo=";

          ldflags = [
            "-s"
            "-w"
            "-X main.version=${self.shortRev or self.dirtyShortRev or "dev"}"
          ];

          meta = with pkgs.lib; {
            description = "Mirror every repository on a self-hosted GitLab or Forgejo instance, or a GitHub.com account, to a local, restorable backup";
            homepage = "https://github.com/alrayyes/backup-git-repos";
            license = licenses.gpl3Only;
            mainProgram = "backup-git-repos";
          };
        };

        apps.default = flake-utils.lib.mkApp {
          drv = self.packages.${system}.default;
        };
      }
    );
}
