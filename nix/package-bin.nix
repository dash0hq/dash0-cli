# Pre-built release binary of the Dash0 CLI.
#
# Repackages the GoReleaser artifact from GitHub Releases instead of compiling
# from source. Use this when you want a fast, compile-free install (small or
# non-x86 machines, CI). The from-source package lives in ./package.nix and
# stays the default; see the README's "Install with Nix" section.
#
# To bump: change `version` and refresh the four `hash` values, e.g.
#   nix store prefetch-file --json \
#     https://github.com/dash0hq/dash0-cli/releases/download/v<version>/dash0_<version>_linux_amd64.tar.gz
{
  lib,
  stdenvNoCC,
  fetchurl,
  installShellFiles,
  version ? "1.15.0",
}:
let
  base = "https://github.com/dash0hq/dash0-cli/releases/download/v${version}";

  # Maps a Nix system to the GoReleaser asset suffix (.goreleaser.yaml names
  # darwin builds "macos") and the tarball's SRI hash.
  sources = {
    "x86_64-linux" = {
      suffix = "linux_amd64";
      hash = "sha256-EFb9l1CxL4tcCfmhEaLnOCSgVyiyw/CWotmOhIZpg+c=";
    };
    "aarch64-linux" = {
      suffix = "linux_arm64";
      hash = "sha256-KBnCgGvAu6Hq4hMqtPlflroUph0MQEwQsNCgj4aPtd4=";
    };
    "x86_64-darwin" = {
      suffix = "macos_amd64";
      hash = "sha256-O6JwOdy4alVnQ4j6hS2LHJj0WC85cSBDNtzi084rPBw=";
    };
    "aarch64-darwin" = {
      suffix = "macos_arm64";
      hash = "sha256-tpxbqrEcwjxU24V9Aai4jGKb0OgBvjpHVx8sWeAwh1c=";
    };
  };
in
stdenvNoCC.mkDerivation (finalAttrs: {
  pname = "dash0";
  inherit version;

  src =
    let
      sys = stdenvNoCC.hostPlatform.system;
      source = sources.${sys} or (throw "dash0-bin: unsupported system '${sys}'");
    in
    fetchurl {
      url = "${base}/dash0_${version}_${source.suffix}.tar.gz";
      inherit (source) hash;
    };

  # The GoReleaser tarball is flat (dash0 + completions/ at the root).
  sourceRoot = ".";

  nativeBuildInputs = [ installShellFiles ];

  # The release binary is built CGO_ENABLED=0 (see .goreleaser.yaml), so it is
  # statically linked and runs on NixOS as-is — no autoPatchelfHook needed.
  dontConfigure = true;
  dontBuild = true;

  installPhase = ''
    runHook preInstall
    install -Dm755 dash0 $out/bin/dash0
    installShellCompletion --cmd dash0 \
      --bash completions/dash0.bash \
      --zsh completions/dash0.zsh \
      --fish completions/dash0.fish
    runHook postInstall
  '';

  # Smoke-test the unpacked binary (skipped when the build host can't run it).
  doInstallCheck = stdenvNoCC.buildPlatform.canExecute stdenvNoCC.hostPlatform;
  installCheckPhase = ''
    $out/bin/dash0 version | grep -q "${version}"
  '';

  meta = {
    description = "CLI to interact with Dash0 (pre-built release binary)";
    homepage = "https://github.com/dash0hq/dash0-cli";
    changelog = "https://github.com/dash0hq/dash0-cli/blob/v${version}/CHANGELOG.md";
    license = lib.licenses.asl20;
    mainProgram = "dash0";
    sourceProvenance = [ lib.sourceTypes.binaryNativeCode ];
    platforms = builtins.attrNames sources;
  };
})
