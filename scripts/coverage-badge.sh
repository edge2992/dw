#!/usr/bin/env bash
# Generate a flat coverage badge SVG from a Go coverage profile.
# Usage: scripts/coverage-badge.sh [coverage.out] [.github/badges/coverage.svg]
set -euo pipefail

profile="${1:-coverage.out}"
out="${2:-.github/badges/coverage.svg}"

pct=$(go tool cover -func="$profile" | awk '/^total:/ {gsub(/%/,"",$3); print $3}')
[ -n "$pct" ] || { echo "coverage-badge: no total coverage found in $profile" >&2; exit 1; }

color=$(awk -v p="$pct" 'BEGIN{
  if (p>=90) c="#4c1";
  else if (p>=75) c="#97ca00";
  else if (p>=50) c="#dfb317";
  else if (p>=25) c="#fe7d37";
  else c="#e05d44";
  print c
}')

val="${pct}%"
lw=61                       # label "coverage" width
vw=$(( 12 + 7 * ${#val} ))  # value width scales with text length
w=$(( lw + vw ))
lx=$(( lw / 2 ))
vx=$(( lw + vw / 2 ))

mkdir -p "$(dirname "$out")"
cat > "$out" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="20" role="img" aria-label="coverage: ${val}">
  <title>coverage: ${val}</title>
  <g shape-rendering="crispEdges">
    <rect width="${lw}" height="20" fill="#555"/>
    <rect x="${lw}" width="${vw}" height="20" fill="${color}"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
    <text x="${lx}" y="14">coverage</text>
    <text x="${vx}" y="14">${val}</text>
  </g>
</svg>
SVG

echo "coverage-badge: ${val} -> ${out}"
