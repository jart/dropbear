# Context: implementation-stage

Context: implementation-stage
Title: Implementation Stage Context
Parent: project

Content:
# Implementation Stage - Build & Test

**You are authorized to write code and make commits. Build it, test it, commit it.**

## Your Mission Right Now

Implement the functionality according to your design. Write tests alongside code. Make incremental commits. When core functionality works and tests pass, transition to comprehensive testing.

## The Flow (Work Continuously)

1. **Build Core Functionality** (main work)
   - Implement according to your architecture plan
   - Write unit tests as you go (not after)
   - Make small, focused commits with clear messages
   - Run tests frequently: `go test ./...`
   - Run benchmarks when relevant: `go test -bench=. ./...`
   - Document progress: `git-codemap note bug/name "implemented X, tests passing"`

2. **Quality Checks** (continuous)
   - Run formatter: `go fmt ./...`
   - Fix issues immediately - don't accumulate tech debt
   - Ensure code follows project patterns
   - Use `decimal/` not floats for financial math
   - Use `clocky.Now()` for mockable time

3. **Actor-Specific Checklist** (for actor work)
   - Message types are well-defined structs
   - Actor handlers have clear input/output types
   - Mailbox capacity is appropriate
   - Tests inject messages directly (no mocking globals)

4. **Proceed Automatically** (when ready)
   - When core functionality works and tests pass, **transition immediately**
   - Use: `git-codemap transition testing`

## Decision Criteria

**✅ Proceed when**: Core functionality implemented, unit tests passing, code is clean
**❌ Don't wait for**: 100% test coverage, perfect refactoring, or approval

## Note-Taking: Be Concise, Be Frequent

Use `git-codemap note bug/name "..."` to document:
- Progress on implementation ("implemented X, tests passing")
- Issues encountered and resolutions
- Design changes from original plan
- Test coverage additions

## Anti-Patterns

- ❌ Using IEEE floats for financial calculations
- ❌ Using `time.Now()` instead of `clocky.Now()`
- ❌ Implementing without tests - **write tests alongside**
- ❌ Waiting to commit until "everything is perfect" - **commit incrementally**
- ✅ Making commits as you complete chunks of work
- ✅ Running tests frequently
- ✅ Using table-driven tests


