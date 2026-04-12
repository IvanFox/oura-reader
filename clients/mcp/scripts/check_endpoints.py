"""Verify that the Python REGISTRY matches the Go endpoint registry.

Exits 0 on match, 1 on drift (with a diff printed to stderr).
Run via `make check-endpoints` from the repo root.
"""

from __future__ import annotations

import re
import sys
from dataclasses import dataclass
from pathlib import Path


GO_FILE = Path(__file__).resolve().parents[3] / "internal" / "oura" / "endpoints.go"


@dataclass(frozen=True)
class GoSpec:
    name: str
    has_dates: bool
    is_list: bool
    has_day_field: bool


_SPEC_RE = re.compile(
    r'\{\s*Name:\s*"(?P<name>[^"]+)",'
    r'.*?HasDates:\s*(?P<has_dates>true|false),'
    r'.*?IsList:\s*(?P<is_list>true|false),'
    r'.*?DayField:\s*"(?P<day_field>[^"]*)"\s*\}',
    re.DOTALL,
)


def parse_go_registry(text: str) -> list[GoSpec]:
    return [
        GoSpec(
            name=m.group("name"),
            has_dates=m.group("has_dates") == "true",
            is_list=m.group("is_list") == "true",
            has_day_field=m.group("day_field") != "",
        )
        for m in _SPEC_RE.finditer(text)
    ]


def main() -> int:
    from oura_mcp.endpoints import REGISTRY

    go_text = GO_FILE.read_text()
    go_specs = parse_go_registry(go_text)

    go_set = {(s.name, s.has_dates, s.is_list, s.has_day_field) for s in go_specs}
    py_set = {(s.name, s.has_dates, s.is_list, s.has_day_field) for s in REGISTRY}

    if go_set == py_set:
        print(f"ok: {len(go_set)} endpoints match")
        return 0

    print("DRIFT between Go and Python registries:", file=sys.stderr)
    for item in sorted(go_set - py_set):
        print(f"  only in Go: {item}", file=sys.stderr)
    for item in sorted(py_set - go_set):
        print(f"  only in Python: {item}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
