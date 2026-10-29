#!/usr/bin/env python3
"""Audit file size, function size and forbidden imports on a source tree.

Functions are found in Go, JavaScript/TypeScript and Python; any other source
file is measured for size only. Standard library only, Python 3.8+.

Exit code 0 = no error, 1 = at least one error (or a warning with --strict).
"""

from __future__ import annotations

import argparse
import fnmatch
import json
import re
import sys
from pathlib import Path

SKIP_DIRS = {".git", "node_modules", "vendor", "dist", "build", "target",
             ".venv", "venv", "__pycache__", ".idea", ".next", "out"}
JS_EXTS = {".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".mts", ".cts"}
PY_EXTS = {".py"}
SOURCE_EXTS = JS_EXTS | PY_EXTS | {".go", ".java", ".kt", ".rb", ".rs", ".cs", ".php", ".swift"}

GO_FUNC = re.compile(r"^func\s+(?:\([^)]*\)\s*)?(\w+)")
JS_DECL = re.compile(r"^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s*(\w+)")
JS_ARROW = re.compile(r"^\s*(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>")
JS_METHOD = re.compile(r"^ {0,6}(?:async\s+)?(?:static\s+)?(?:get\s+|set\s+)?([A-Za-z_$][\w$]*)\s*\([^;]*\)\s*\{\s*$")
JS_KEYWORDS = {"if", "for", "while", "switch", "catch", "function", "return", "do", "else", "new", "with"}
PY_DEF = re.compile(r"^(\s*)(?:async\s+)?def\s+(\w+)")
GO_IMPORT = re.compile(r'^\s*(?:[\w.]+\s+)?"([^"]+)"\s*$')
JS_IMPORT = re.compile(r"""(?:from\s+|require\(\s*)['"]([^'"]+)['"]""")
PY_IMPORT = re.compile(r"^\s*(?:from\s+([\w.]+)\s+import|import\s+([\w.]+))")


class Finding:
    """One rule violation, or a warning close to one."""

    def __init__(self, level, rule, where, message):
        self.level, self.rule, self.where, self.message = level, rule, where, message

    def as_dict(self):
        return {"level": self.level, "rule": self.rule, "where": self.where, "message": self.message}

    def render(self):
        return f"{self.level:<7} {self.rule:<15} {self.where:<40} {self.message}"


def is_binary(path: Path) -> bool:
    try:
        return b"\x00" in path.open("rb").read(8192)
    except OSError:
        return True


def source_files(paths, excludes) -> list[Path]:
    """Every readable source file under the given paths."""
    found: list[Path] = []
    for raw in paths:
        root = Path(raw)
        candidates = [root] if root.is_file() else sorted(root.rglob("*"))
        for path in candidates:
            if not path.is_file() or path.suffix not in SOURCE_EXTS:
                continue
            if SKIP_DIRS & set(path.parts):
                continue
            if any(fnmatch.fnmatch(str(path), pat) or pat in path.parts for pat in excludes):
                continue
            if is_binary(path):
                continue
            found.append(path)
    return found


def read_lines(path: Path) -> list[str]:
    return path.read_text(encoding="utf-8", errors="replace").splitlines()


def clean_lines(lines: list[str], hash_comments=False) -> list[str]:
    """Drop comments and literals, including the ones that span several lines,
    so that a brace inside a string never moves a function's end."""
    out, quote, block = [], None, False
    for line in lines:
        buf, i = [], 0
        while i < len(line):
            ch = line[i]
            if quote:
                if ch == "\\":
                    i += 2
                    continue
                quote = None if ch == quote else quote
                i += 1
                continue
            if block:
                block = not line.startswith("*/", i)
                i += 1 if block else 2
                continue
            if line.startswith("//", i) or (hash_comments and ch == "#"):
                break
            if line.startswith("/*", i):
                block, i = True, i + 2
                continue
            quote = ch if ch in "\"'`" else quote
            i += 1
            if not quote:
                buf.append(ch)
        # A quoted literal cannot span lines; a backtick one can. Forgetting
        # this would let one apostrophe inside a regex swallow the file.
        quote = quote if quote == "`" else None
        out.append("".join(buf))
    return out


def brace_span(lines: list[str], start: int) -> int:
    """Last line of the block whose opening brace is at or after start."""
    depth, opened = 0, False
    for i in range(start, len(lines)):
        body = lines[i]
        depth += body.count("{") - body.count("}")
        opened = opened or "{" in body
        if opened and depth <= 0:
            return i
    return len(lines) - 1


def js_name(line: str):
    for pattern in (JS_DECL, JS_ARROW, JS_METHOD):
        m = pattern.match(line)
        if m and m.group(1) not in JS_KEYWORDS:
            return m.group(1)
    return None


def js_functions(lines: list[str]):
    clean = clean_lines(lines)
    for i, line in enumerate(clean):
        name = js_name(line)
        if name:
            yield name, i, brace_span(clean, i)


def go_functions(lines: list[str]):
    clean = clean_lines(lines)
    return [(GO_FUNC.match(l).group(1), i, brace_span(clean, i))
            for i, l in enumerate(clean) if GO_FUNC.match(l)]


def py_functions(lines: list[str]):
    for i, line in enumerate(lines):
        m = PY_DEF.match(line)
        if not m:
            continue
        indent = len(m.group(1))
        end = i
        for j in range(i + 1, len(lines)):
            if not lines[j].strip() or lines[j].lstrip().startswith("#"):
                continue
            if len(lines[j]) - len(lines[j].lstrip()) <= indent:
                break
            end = j
        yield m.group(2), i, end


def functions_of(path: Path, lines: list[str]):
    suffix = path.suffix
    if suffix == ".go":
        return go_functions(lines)
    if suffix in JS_EXTS:
        return list(js_functions(lines))
    if suffix in PY_EXTS:
        return list(py_functions(lines))
    return []


def size_findings(path, lines, args) -> list[Finding]:
    findings = []
    count = len(lines)
    if count > args.max_file:
        findings.append(Finding("ERROR", "file-size", str(path), f"{count} lines (max {args.max_file})"))
    elif count >= args.warn_ratio * args.max_file:
        findings.append(Finding("WARNING", "file-size", str(path), f"{count} lines ({round(count * 100 / args.max_file)}% of {args.max_file})"))
    for name, start, end in functions_of(path, lines):
        span = end - start + 1
        where = f"{path}:{start + 1}"
        if span > args.max_function:
            findings.append(Finding("ERROR", "function-size", where, f"{name}: {span} lines (max {args.max_function})"))
        elif span >= args.warn_ratio * args.max_function:
            findings.append(Finding("WARNING", "function-size", where, f"{name}: {span} lines ({round(span * 100 / args.max_function)}% of {args.max_function})"))
    return findings


def imports_of(path: Path, lines: list[str]) -> list[str]:
    suffix = path.suffix
    out = []
    for line in lines:
        if suffix == ".go":
            m = GO_IMPORT.match(line)
            out += [m.group(1)] if m else []
        elif suffix in JS_EXTS:
            out += JS_IMPORT.findall(line)
        elif suffix in PY_EXTS:
            m = PY_IMPORT.match(line)
            out += [group for group in m.groups() if group] if m else []
    return out


def forbid_rules(specs) -> list[tuple[str, str]]:
    rules = []
    for spec in specs:
        scope, _, names = spec.partition("=")
        for name in names.split(","):
            if scope and name.strip():
                rules.append((scope.strip(), name.strip()))
    return rules


def under(path: Path, scope: str) -> bool:
    """Whether a file sits under a scope, however the paths were written."""
    scope = scope.strip("/")
    written, absolute = path.as_posix(), path.resolve().as_posix()
    return (f"/{scope}/" in f"/{absolute}" or written == scope
            or written.startswith(scope + "/") or fnmatch.fnmatch(written, scope)
            or fnmatch.fnmatch(absolute, "*/" + scope))


def import_findings(path, lines, rules) -> list[Finding]:
    findings = []
    for scope, forbidden in rules:
        if not under(path, scope):
            continue
        hits = [imp for imp in imports_of(path, lines) if imp == forbidden or imp.startswith(forbidden + "/")]
        if hits:
            findings.append(Finding("ERROR", "dependency", str(path), f"imports {hits[0]} (forbidden under {scope})"))
    return findings


def render_declarations(found, args):
    ranked = sorted(found, key=lambda item: -item[1])[: args.top]
    for path, count, funcs in ranked:
        print(f"\n{path} - {count} lines, {len(funcs)} function(s)")
        for name, start, end in funcs[: args.top]:
            print(f"  {start + 1:>5}  {end - start + 1:>4} lines  {name}")


def parse_args(argv):
    parser = argparse.ArgumentParser(
        description="Audit file size, function size and forbidden imports.",
        epilog="Example: check_architecture.py src --forbid 'src/domain=net/http,database/sql'",
    )
    parser.add_argument("paths", nargs="*", default=["."], help="files or directories to audit")
    parser.add_argument("--max-file", type=int, default=300, help="max lines per file (default 300)")
    parser.add_argument("--max-function", type=int, default=40, help="max lines per function (default 40)")
    parser.add_argument("--warn-ratio", type=float, default=0.8, help="warning threshold as a ratio of the max (default 0.8)")
    parser.add_argument("--exclude", action="append", default=[], help="glob or directory name to skip")
    parser.add_argument("--forbid", action="append", default=[], help="PATH_GLOB=IMPORT[,IMPORT]: imports forbidden under a path")
    parser.add_argument("--json", action="store_true", help="emit a JSON report")
    parser.add_argument("--declarations", action="store_true", help="list the function spans of the largest files")
    parser.add_argument("--top", type=int, default=20, help="files listed by --declarations (default 20)")
    parser.add_argument("--strict", action="store_true", help="exit 1 on warnings too")
    return parser.parse_args(argv)


def summarize(found, args) -> int:
    findings = []
    declarations = []
    for path, lines, funcs in found:
        findings += size_findings(path, lines, args) + import_findings(path, lines, forbid_rules(args.forbid))
        declarations.append((path, len(lines), funcs))
    findings.sort(key=lambda f: (f.level != "ERROR", f.rule, f.where))
    errors = sum(1 for f in findings if f.level == "ERROR")
    warnings = len(findings) - errors
    if args.json:
        print(json.dumps({
            "files": [{"path": str(p), "lines": count} for p, count, _ in declarations],
            "findings": [f.as_dict() for f in findings],
            "errors": errors, "warnings": warnings,
        }, indent=2, ensure_ascii=False))
    else:
        print(f"Clean Architecture audit - {len(declarations)} files, "
              f"max {args.max_file} lines/file, {args.max_function} lines/function\n")
        for finding in findings:
            print(finding.render())
        print(f"\nSummary: {errors} error(s), {warnings} warning(s), {len(declarations)} file(s) scanned")
        if args.declarations:
            render_declarations(declarations, args)
    return 1 if errors or (args.strict and warnings) else 0


def main(argv=None) -> int:
    args = parse_args(argv)
    found = []
    for path in source_files(args.paths, args.exclude):
        lines = read_lines(path)
        found.append((path, lines, functions_of(path, lines)))
    return summarize(found, args)


if __name__ == "__main__":
    sys.exit(main())
