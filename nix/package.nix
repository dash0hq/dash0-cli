# Nix derivation for the Dash0 CLI.
#
# This file is consumed both by `flake.nix` (via `callPackage`) and by the
# non-flake `default.nix` shim, so it stays free of flake-specific plumbing and
# takes everything it needs as arguments.
{
  lib,
  buildGoModule,
  installShellFiles,
  stdenv,
  # Build metadata. `flake.nix` overrides these with values derived from the
  # flake source; the defaults keep a bare `nix-build ./nix/package.nix` working.
  version ? "1.15.0",
  date ? "1970-01-01T00:00:00Z",
}:

buildGoModule (finalAttrs: {
  pname = "dash0";
  inherit version;

  # Build from the repository root. The filter keeps the Nix store copy small
  # and reproducible by dropping build outputs, VCS metadata, and editor cruft
  # that do not influence the compiled binary.
  src =
    let
      root = ../.;
    in
    lib.cleanSourceWith {
      src = lib.cleanSource root;
      filter =
        path: type:
        let
          rel = lib.removePrefix (toString root + "/") (toString path);
        in
        !(lib.hasPrefix "build/" rel)
        && !(lib.hasPrefix "dist/" rel)
        && !(lib.hasPrefix ".tools/" rel)
        && !(lib.hasPrefix "completions/" rel)
        && !(lib.hasPrefix ".git/" rel);
    };

  # Hash of the fetched Go module dependencies. The placeholder below must be
  # replaced with the real hash: build once, then copy the `got: sha256-...`
  # value from the resulting hash-mismatch error into this field. Re-run after
  # any change to go.mod / go.sum. See the README's "Install with Nix" section.
  vendorHash = "sha256-LB1GblViRropaKmWmQjx3qN1L5O9zLqP/uLxRj9aVOM=";

  # Only the CLI entrypoint is a `main` package; building it explicitly avoids
  # compiling test-only helpers into the output.
  subPackages = [ "cmd/dash0" ];

  # Mirror the ldflags used by GoReleaser (.goreleaser.yaml) so the `version`
  # command and the HTTP User-Agent report the right build.
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
    "-X main.date=${date}"
  ];

  # The unit test suite is hermetic (the network-dependent integration and
  # roundtrip suites are gated behind the `integration` build tag and shell
  # scripts, neither of which run here).
  doCheck = true;

  nativeBuildInputs = [ installShellFiles ];

  # Generate and install shell completions from the built binary, matching what
  # `scripts/completions.sh` and the Homebrew cask ship. Skipped when
  # cross-compiling, where the freshly built binary cannot be executed.
  postInstall = lib.optionalString (stdenv.buildPlatform.canExecute stdenv.hostPlatform) ''
    installShellCompletion --cmd dash0 \
      --bash <($out/bin/dash0 completion bash) \
      --zsh <($out/bin/dash0 completion zsh) \
      --fish <($out/bin/dash0 completion fish)
  '';

  meta = {
    description = "CLI to interact with Dash0";
    longDescription = ''
      The Dash0 CLI manages Dash0 assets (dashboards, views, check rules,
      synthetic checks, recording rules, notification channels, and spam
      filters), queries telemetry (logs, spans, traces, metrics, failed
      checks), and sends signals via OTLP. It is built for use from the
      terminal, CI/CD pipelines, and AI coding agents.
    '';
    homepage = "https://github.com/dash0hq/dash0-cli";
    changelog = "https://github.com/dash0hq/dash0-cli/blob/v${version}/CHANGELOG.md";
    license = lib.licenses.asl20;
    mainProgram = "dash0";
  };
})
