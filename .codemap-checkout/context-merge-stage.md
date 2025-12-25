# Context: merge-stage

Context: merge-stage
Title: Merge Stage Context
Parent: project

Content:
# Merge Stage - Final Integration

**You are authorized to prepare for integration. Check rebase status, verify quality, mark complete.**

## Your Mission Right Now

Final verification before marking ready for integration. Check if rebase is needed, ensure all tests still pass, then mark as complete for mergemeister integration.

## The Flow (Work Continuously)

1. **Check Integration Status** (verify)
   - Check for upstream changes: `git-codemap check-rebase`
   - Review status: `git-codemap bug status`
   - Check for conflicts with other active bugs

2. **Rebase If Needed** (if upstream changed)
   - If upstream has new commits: `git rebase upstream`
   - Resolve any conflicts carefully
   - **Re-run full test suite** after rebase
   - Ensure quality checks still pass
   - Document: `git-codemap note bug/name "rebased on upstream, tests still pass"`

3. **Final Verification** (thorough)
   - Run complete test suite one final time
   - Run all quality checks (linter, formatter)
   - Review your changes: `git diff upstream`
   - Verify acceptance criteria still met

4. **Mark Complete** (automatic)
   - All tests passing after rebase? ✅
   - Quality checks clean? ✅
   - No conflicts? ✅
   - **Mark complete immediately**: `git-codemap transition complete`
   - Mergemeister (separate LLM thread) handles actual integration

## Decision Criteria

**✅ Mark complete when**: Tests pass, quality clean, rebased if needed, no conflicts
**❌ Don't wait for**: Manual approval, code review, or permission to mark complete

## What Mergemeister Does

Mergemeister is a separate LLM thread that handles integration:
- Reviews completed bugs
- Merges to upstream when appropriate
- Handles any final integration issues
- **Your job**: Get tests passing, mark complete, let mergemeister integrate

## Note-Taking: Be Concise, Be Frequent

Use `git-codemap note bug/name "..."` to document:
- Rebase status ("rebased on upstream, no conflicts")
- Final test results after rebase
- Any integration issues discovered
- Ready-for-merge confirmation

Keep notes short (1-2 sentences) but add them frequently.

## Anti-Patterns

- ❌ Asking "Should I check for rebase?" - **check automatically**
- ❌ Skipping rebase "because it's probably fine" - **always check**
- ❌ Marking complete with failing tests - **tests must pass**
- ❌ Trying to merge yourself - **let mergemeister handle it**
- ❌ Waiting for permission to mark complete - **just mark it**
- ✅ Checking rebase status automatically
- ✅ Re-running tests after rebase
- ✅ Documenting final status with notes
- ✅ Marking complete when quality gates pass
- ✅ Trusting mergemeister to handle integration


