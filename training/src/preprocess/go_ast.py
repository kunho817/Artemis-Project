"""Go AST extraction utilities for code-generation training data."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import tree_sitter
import tree_sitter_go as tsgo
from tqdm import tqdm


class GoASTExtractor:
    """Extract structured Go symbols from source files using tree-sitter."""

    def __init__(self, config: dict[str, Any]) -> None:
        """Initialize extractor from AST config section.

        Args:
            config: AST configuration dictionary.
        """
        self.config = config
        self.min_function_lines = int(config.get("min_function_lines", 3))
        self.max_function_lines = int(config.get("max_function_lines", 200))

        GO_LANGUAGE = tsgo.language()
        self.parser = tree_sitter.Parser(tree_sitter.Language(GO_LANGUAGE))

    def extract_from_file(self, content: str, file_path: str) -> list[dict[str, Any]]:
        """Parse Go source and extract functions, methods, types, and interfaces.

        Args:
            content: Full Go source text.
            file_path: Source file path.

        Returns:
            Extracted symbol records.
        """
        source_bytes = content.encode("utf-8")
        tree = self.parser.parse(source_bytes)
        root = tree.root_node
        imports = self._extract_imports(root, source_bytes)
        lines = content.splitlines()

        items: list[dict[str, Any]] = []
        self._walk(root, source_bytes, lines, imports, file_path, items)
        return items

    def process_jsonl(self, input_path: str, output_path: str) -> int:
        """Extract symbols from raw-file JSONL and write symbol JSONL.

        Expected input schema includes at least `content` and optionally `path`.

        Args:
            input_path: Raw file JSONL path.
            output_path: Output JSONL path for extracted symbols.

        Returns:
            Number of extracted symbol items written.
        """
        in_path = Path(input_path)
        out_path = Path(output_path)
        out_path.parent.mkdir(parents=True, exist_ok=True)

        written = 0
        with in_path.open("r", encoding="utf-8") as infile, out_path.open(
            "w", encoding="utf-8"
        ) as outfile:
            for raw_line in tqdm(infile, desc="Extracting Go AST", unit="file"):
                line = raw_line.strip()
                if not line:
                    continue

                try:
                    record = json.loads(line)
                except json.JSONDecodeError:
                    continue

                content = record.get("content")
                if not isinstance(content, str) or not content:
                    continue

                file_path = str(record.get("path") or "")
                extracted = self.extract_from_file(content=content, file_path=file_path)
                for symbol in extracted:
                    outfile.write(json.dumps(symbol, ensure_ascii=False) + "\n")
                    written += 1

        return written

    def _walk(
        self,
        node: tree_sitter.Node,
        source_bytes: bytes,
        lines: list[str],
        imports: list[str],
        file_path: str,
        output: list[dict[str, Any]],
    ) -> None:
        if node.type == "function_declaration":
            item = self._extract_function_like(
                node=node,
                source_bytes=source_bytes,
                lines=lines,
                imports=imports,
                file_path=file_path,
                kind="function",
            )
            if item is not None:
                output.append(item)
        elif node.type == "method_declaration":
            item = self._extract_function_like(
                node=node,
                source_bytes=source_bytes,
                lines=lines,
                imports=imports,
                file_path=file_path,
                kind="method",
            )
            if item is not None:
                output.append(item)
        elif node.type == "type_declaration":
            output.extend(
                self._extract_types_from_declaration(
                    node=node,
                    source_bytes=source_bytes,
                    lines=lines,
                    imports=imports,
                    file_path=file_path,
                )
            )

        for child in node.children:
            self._walk(child, source_bytes, lines, imports, file_path, output)

    def _extract_imports(self, root: tree_sitter.Node, source_bytes: bytes) -> list[str]:
        imports: list[str] = []
        stack = [root]
        while stack:
            node = stack.pop()
            if node.type == "import_spec":
                path_node = node.child_by_field_name("path")
                if path_node is not None:
                    raw = source_bytes[path_node.start_byte : path_node.end_byte].decode(
                        "utf-8", errors="replace"
                    )
                    imports.append(raw.strip().strip('"'))
            stack.extend(node.children)
        return imports

    def _extract_function_like(
        self,
        node: tree_sitter.Node,
        source_bytes: bytes,
        lines: list[str],
        imports: list[str],
        file_path: str,
        kind: str,
    ) -> dict[str, Any] | None:
        start_line = node.start_point[0] + 1
        end_line = node.end_point[0] + 1
        line_count = end_line - start_line + 1
        if line_count < self.min_function_lines or line_count > self.max_function_lines:
            return None

        name_node = node.child_by_field_name("name")
        body_node = node.child_by_field_name("body")

        name = (
            source_bytes[name_node.start_byte : name_node.end_byte].decode("utf-8")
            if name_node is not None
            else "<anonymous>"
        )

        full_text = source_bytes[node.start_byte : node.end_byte].decode(
            "utf-8", errors="replace"
        )
        if body_node is not None:
            signature = source_bytes[node.start_byte : body_node.start_byte].decode(
                "utf-8", errors="replace"
            ).strip()
            body = source_bytes[body_node.start_byte : body_node.end_byte].decode(
                "utf-8", errors="replace"
            ).strip()
        else:
            signature = full_text.strip()
            body = ""

        docstring = self._extract_leading_comment(lines=lines, decl_start_line=start_line)

        return {
            "kind": kind,
            "name": name,
            "signature": signature,
            "body": body,
            "docstring": docstring,
            "imports": imports,
            "file_path": file_path,
            "start_line": start_line,
            "end_line": end_line,
        }

    def _extract_types_from_declaration(
        self,
        node: tree_sitter.Node,
        source_bytes: bytes,
        lines: list[str],
        imports: list[str],
        file_path: str,
    ) -> list[dict[str, Any]]:
        out: list[dict[str, Any]] = []
        for child in node.children:
            if child.type != "type_spec":
                continue

            name_node = child.child_by_field_name("name")
            type_node = child.child_by_field_name("type")
            if name_node is None:
                continue

            kind = "interface" if (type_node is not None and type_node.type == "interface_type") else "type"
            start_line = child.start_point[0] + 1
            end_line = child.end_point[0] + 1

            name = source_bytes[name_node.start_byte : name_node.end_byte].decode(
                "utf-8", errors="replace"
            )
            signature = source_bytes[child.start_byte : child.end_byte].decode(
                "utf-8", errors="replace"
            ).strip()
            body = signature
            docstring = self._extract_leading_comment(lines=lines, decl_start_line=start_line)

            out.append(
                {
                    "kind": kind,
                    "name": name,
                    "signature": signature,
                    "body": body,
                    "docstring": docstring,
                    "imports": imports,
                    "file_path": file_path,
                    "start_line": start_line,
                    "end_line": end_line,
                }
            )

        return out

    @staticmethod
    def _extract_leading_comment(lines: list[str], decl_start_line: int) -> str:
        idx = decl_start_line - 2
        collected: list[str] = []

        while idx >= 0:
            current = lines[idx].strip()
            if current.startswith("//"):
                collected.append(current[2:].strip())
                idx -= 1
                continue
            if current == "":
                break
            break

        collected.reverse()
        return "\n".join(collected).strip()
