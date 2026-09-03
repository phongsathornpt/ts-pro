#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";

const targetDirs = ["src", "types"];
const fileExtensions = [".ts", ".d.ts"];

function collectFiles(dir) {
  const results = [];
  if (!fs.existsSync(dir)) return results;
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      results.push(...collectFiles(fullPath));
    } else if (fileExtensions.some(ext => entry.name.endsWith(ext))) {
      results.push(fullPath);
    }
  }
  return results;
}

function stripCommentsAndStrings(source) {
  // Replace comments and strings with whitespace to preserve exact line and column positions
  let result = "";
  let i = 0;
  const len = source.length;

  while (i < len) {
    // Single-line comment
    if (source[i] === "/" && source[i + 1] === "/") {
      while (i < len && source[i] !== "\n") {
        result += " ";
        i++;
      }
      continue;
    }

    // Multi-line comment
    if (source[i] === "/" && source[i + 1] === "*") {
      result += "  ";
      i += 2;
      while (i < len && !(source[i] === "*" && source[i + 1] === "/")) {
        result += source[i] === "\n" ? "\n" : " ";
        i++;
      }
      if (i < len) {
        result += "  ";
        i += 2;
      }
      continue;
    }

    // Double-quoted string
    if (source[i] === '"') {
      result += " ";
      i++;
      while (i < len && source[i] !== '"') {
        if (source[i] === "\\" && i + 1 < len) {
          result += "  ";
          i += 2;
        } else {
          result += source[i] === "\n" ? "\n" : " ";
          i++;
        }
      }
      if (i < len) {
        result += " ";
        i++;
      }
      continue;
    }

    // Single-quoted string
    if (source[i] === "'") {
      result += " ";
      i++;
      while (i < len && source[i] !== "'") {
        if (source[i] === "\\" && i + 1 < len) {
          result += "  ";
          i += 2;
        } else {
          result += source[i] === "\n" ? "\n" : " ";
          i++;
        }
      }
      if (i < len) {
        result += " ";
        i++;
      }
      continue;
    }

    // Template literal (backticks)
    if (source[i] === "`") {
      result += " ";
      i++;
      while (i < len && source[i] !== "`") {
        if (source[i] === "\\" && i + 1 < len) {
          result += "  ";
          i += 2;
        } else if (source[i] === "$" && source[i + 1] === "{") {
          // Template interpolation start: emit normal code
          result += "  ";
          i += 2;
          let braceDepth = 1;
          while (i < len && braceDepth > 0) {
            if (source[i] === "{") braceDepth++;
            else if (source[i] === "}") braceDepth--;
            result += source[i];
            i++;
          }
        } else {
          result += source[i] === "\n" ? "\n" : " ";
          i++;
        }
      }
      if (i < len) {
        result += " ";
        i++;
      }
      continue;
    }

    result += source[i];
    i++;
  }

  return result;
}

function checkFile(filePath) {
  const rawSource = fs.readFileSync(filePath, "utf-8");
  const strippedSource = stripCommentsAndStrings(rawSource);
  const rawLines = rawSource.split("\n");
  const violations = [];

  const anyRegex = /\bany\b/g;
  let match;

  while ((match = anyRegex.exec(strippedSource)) !== null) {
    const pos = match.index;
    // Calculate line and column
    const linesBefore = strippedSource.slice(0, pos).split("\n");
    const lineNum = linesBefore.length;
    const colNum = linesBefore[linesBefore.length - 1].length + 1;
    const lineSnippet = rawLines[lineNum - 1] ?? "";

    violations.push({
      filePath,
      lineNum,
      colNum,
      lineSnippet,
    });
  }

  return violations;
}

function main() {
  const allFiles = targetDirs.flatMap(collectFiles);
  let totalViolations = 0;

  for (const file of allFiles) {
    const violations = checkFile(file);
    for (const v of violations) {
      console.error(
        `\x1b[31m${v.filePath}:${v.lineNum}:${v.colNum}: error: explicit 'any' is prohibited. Use 'unknown', generics, or explicit types instead.\x1b[0m`
      );
      console.error(`  ${v.lineNum} | ${v.lineSnippet}`);
      console.error(`     | ${" ".repeat(Math.max(0, v.colNum - 1))}^^^`);
      totalViolations++;
    }
  }

  if (totalViolations > 0) {
    console.error(`\n\x1b[31mFound ${totalViolations} prohibited 'any' usage(s). Strict typing requires banning 'any'.\x1b[0m\n`);
    process.exit(1);
  }

  console.log(`\x1b[32m✓ Checked ${allFiles.length} files in [${targetDirs.join(", ")}]: 0 'any' violations found.\x1b[0m`);
  process.exit(0);
}

main();
