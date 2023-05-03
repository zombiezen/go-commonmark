{ stdenv
, python3Minimal
, fetchFromGitHub
, lib
}:

let
  version = "0.29.0.gfm.11";
in

stdenv.mkDerivation {
  name = "gfm-spec-${version}.json";
  inherit version;

  src = fetchFromGitHub {
    owner = "github";
    repo = "cmark-gfm";
    rev = version;
    hash = "sha256-2jkMJcfcOH5qYP13qS5Hutbyhhzq9WlqlkthmQoJoCM=";
  };

  nativeBuildInputs = [ python3Minimal ];

  dontConfigure = true;
  buildPhase = ''
    python3 test/spec_tests.py \
      --spec test/spec.txt \
      --dump-tests > spec.json
  '';

  installPhase = ''
    cp spec.json "$out"
  '';

  meta = {
    description = "Test cases for the GitHub Flavored Markdown specification";
    homepage = "https://github.github.com/gfm/";
    license = lib.licenses.cc-by-sa-40;
  };
}
