#!/usr/bin/env bash
# Builds site/reports/ from the Go job's junit.xml and coverage.out: JUnit XML
# for the tests, then Cobertura XML, the native profile and an HTML view for
# coverage, plus an index listing them with the commit and date
# (rules/published-reports.md). CI runs it on pull requests too, so a broken
# conversion fails before the merge; only the upload and deploy are
# main-only.
set -euo pipefail

out="${1:-site/reports}"
sha="${GITHUB_SHA:-$(git rev-parse HEAD)}"
date="$(date -u +%Y-%m-%d)"

rm -rf "$out"
mkdir -p "$out/tests" "$out/coverage"

cp junit.xml "$out/tests/junit.xml"

cp coverage.out "$out/coverage/coverage.out"
go tool gocover-cobertura <coverage.out >"$out/coverage/coverage.xml"
go tool cover -html=coverage.out -o "$out/coverage/index.html"

cat >"$out/index.html" <<HTML
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>backup-git-repos reports</title>
<style>
  :root { color-scheme: light dark; }
  body { font-family: system-ui, sans-serif; max-width: 40rem; margin: 2rem auto; padding: 0 1rem; line-height: 1.5; }
</style>
</head>
<body>
<main>
<h1>backup-git-repos reports</h1>
<p>Commit <code>${sha:0:7}</code>, ${date}.</p>
<ul>
  <li><a href="tests/junit.xml">Test results (JUnit XML)</a></li>
  <li><a href="coverage/">Coverage (HTML)</a></li>
  <li><a href="coverage/coverage.xml">Coverage (Cobertura XML)</a></li>
  <li><a href="coverage/coverage.out">Coverage (Go profile)</a></li>
</ul>
</main>
</body>
</html>
HTML
