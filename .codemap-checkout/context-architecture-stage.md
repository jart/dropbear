# Context: architecture-stage

Context: architecture-stage
Title: Architecture Stage Context
Parent: project

Content:
# Architecture Stage - Design & Plan

**You are authorized to make design decisions. Design it and move forward.**

## Your Mission Right Now

Design the technical approach. Identify message types, actor boundaries, morphisms, and tests. Document decisions concisely. Then transition to implementation.

## The Flow (Work Continuously)

1. **Study Patterns** (5-10 min)
   - Search the codebase for similar patterns
   - Reference havequick: `~/src/havequick/platform/runtime/actor.zig`
   - Identify which files and modules need changes
   - Note findings: `git-codemap note bug/name "files: actor/mailbox.go"`

2. **Actor Decomposition** (for actor work)
   - Define message types (structs with clear fields)
   - Identify actor boundaries (what state is encapsulated?)
   - Map message flow (A → B → C)
   - Identify morphisms (input type → output type)
   - Note decisions: `git-codemap note bug/name "SpreadActor: BinanceTick+CoinbaseTick → SpreadSignal"`

3. **Categorical Properties** (for ISO work)
   - Identify monoid/group structure (identity? inverse? associative?)
   - Define projections (Go type ⇄ Core type)
   - Plan law verification tests
   - Note: `git-codemap note bug/name "Spread forms monoid with zero as identity"`

4. **Document & Proceed** (NOW)
   - Document your plan in 3-5 bullet points as notes
   - **Transition immediately** to implementation
   - Use: `git-codemap transition implementation`

## Decision Criteria

**✅ Proceed when**: You know message types, actor boundaries, and test approach
**❌ Don't wait for**: Perfect design or extensive documentation

## Anti-Patterns

- ❌ Over-designing for hypothetical future requirements
- ❌ Creating extensive architecture documents
- ❌ Waiting for review before starting implementation
- ✅ Making pragmatic technical decisions
- ✅ Documenting key choices with concise notes
- ✅ Transitioning as soon as you have a plan


