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

          nativeBuildInputs = [ pkgs.installShellFiles ];

          vendorHash = "sha256-MhWXB/aLsJQhC8NAcb6fCQTtJxEAU14l5ieiME2wgA0=";

          ldflags = [
            "-s"
            "-w"
            "-X main.version=${self.shortRev or self.dirtyShortRev or "dev"}"
          ];

          # The hidden `man` command writes the pages from the freshly built
          # binary, the same way goreleaser's before.hooks does.
          postInstall = ''
            $out/bin/backup-git-repos man man
            installManPage man/*.1
          '';

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
