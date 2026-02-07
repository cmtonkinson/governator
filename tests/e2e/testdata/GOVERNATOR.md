# Toy `ls` Clone in C

Implement a small C program (`toyls`) that lists the contents of a directory. It accepts an optional path argument; if
omitted, it lists the current working directory. Output must list **directories first**, then **files**, and for each
entry show:

- **Permissions** (e.g., `drwxr-xr-x`)
- **Creation time**
- **Last modified time**
- **Name**

This is a "toy" implementation: correct, readable, and portable-ish, not a full GNU `ls`.

## Goals
- `./toyls [path]` works on a POSIX-ish system.
- Prints **directories first**, then **files** (symlinks treated as "files" unless explicitly decided otherwise).
- Prints a stable, easy-to-read table format.

## Non-goals
- No flags other than optional `path` argument.
- No recursion, no color, no inode numbers, no hardlink counts, no user/group names, no device major/minor, no humanized
  sizes.
- No terminal-width column packing; one entry per line is fine.
- No locale-aware sorting (ASCII sort is fine).

## Functional Requirements

### CLI behavior
- `toyls` with no args lists `.` (cwd).
- `toyls /some/path` lists that directory.
- If the path is not a directory (or cannot be opened), return non-zero and print a helpful error message to stderr.

### Listing semantics
- Iterate directory entries.
- Skip `.` and `..` (always).
- Partition results into:
  1. directories
  2. non-directories

### Output format
One entry per line, with fixed columns:
- **Permissions** (e.g., `drwxr-xr-x`)
- **Creation time**
- **Last modified time**
- **Name**
