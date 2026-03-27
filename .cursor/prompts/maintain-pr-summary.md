# Maintain PR Summary

This prompt helps maintain a running PR summary document that tracks changes as work is developed on a branch.

## Usage

Type `@maintain-pr-summary` in Cursor, then:
- "Update the PR summary with the changes we just made"
- "Create a new PR summary for this branch"
- "Add the bug fix to the PR summary"

## Options

### Default Behavior
- **Location**: Current working directory (project root)
- **Filename**: `PR-SUMMARY-<branch-name>.md` (e.g., `PR-SUMMARY-add-e2e-tests.md`)
- **Gitignored**: Yes (file is automatically ignored)

### Custom Location
Specify a full file path to save the summary elsewhere:
- "Update the PR summary at `~/workspaces/my-pr-summary.md`"

## What Gets Tracked

### Header
- Branch name and target branch
- Date of last update
- High-level summary of changes

### Sections
1. **Summary** - Brief description of the PR's purpose with key changes list
2. **Commits** - Table of commits with descriptions
3. **Files Changed** - Statistics and categorized tables (new, modified, deleted)
4. **Bug Fixes** - Detailed problem/solution descriptions with affected files
5. **CI Impact** - How changes affect CI runs (ShellCheck, etc.)
6. **Checklist** - Completion status of major items

## Template Structure

```markdown
# PR Summary: <Title>

**Branch**: `<branch-name>`
**Target**: `main`
**Last Updated**: <YYYY-MM-DD>

---

## Summary

<Brief description of what this PR accomplishes>

### Key Changes

1. **Category 1**: Description
2. **Category 2**: Description

---

## Commits

| Commit | Description |
|--------|-------------|
| `abc1234` | Commit message summary |
| *(pending)* | Uncommitted changes description |

---

## Files Changed (X files, +Y / -Z lines)

### New Files Created

| File | Lines | Purpose |
|------|-------|---------|
| `path/to/file.sh` | 100 | Description |

### Files Modified

| File | Change |
|------|--------|
| `path/to/file.sh` | +50/-20 lines - Description of changes |

---

## Bug Fixes

### 1. Issue Title

**Problem**: Description of the bug or issue.

**Solution**: How it was fixed, including approach taken.

**Files Changed**:
- `file1.sh`
- `file2.sh`

---

## CI Impact

- **Impact 1**: Description of how this affects CI
- **ShellCheck**: Any new scripts added to lint scope

---

## Checklist

- [x] Completed item
- [ ] Pending item
- [ ] ShellCheck passes locally
- [ ] CLAUDE.md updated if needed
- [ ] README.md updated if needed
```

## Notes

- The summary file is gitignored by default to avoid cluttering PRs
- Updates are additive — new changes are appended to existing sections
- Run `git diff --stat main` to get accurate line change counts
- The summary can be copied into the actual PR description when ready
