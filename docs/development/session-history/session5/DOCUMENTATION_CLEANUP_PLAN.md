# Documentation Cleanup Plan

## Current State
The root directory contains 40+ documentation files that need organization. This makes it hard to find relevant information and creates a poor first impression for new users.

## Proposed Structure

```
dynamorm/
├── README.md                    # Main readme (keep in root)
├── LICENSE                      # License file (keep in root)
├── CONTRIBUTING.md             # Contributing guide (keep in root)
├── .gitignore
├── go.mod
├── go.sum
├── Makefile
├── dynamorm.go                 # Main package file
├── lambda.go                   # Lambda optimizations (new)
├── multiaccount.go            # Multi-account support (new)
│
├── docs/                       # All documentation
│   ├── README.md              # Documentation index
│   │
│   ├── getting-started/       # For new users
│   │   ├── installation.md
│   │   ├── quickstart.md
│   │   ├── basic-usage.md
│   │   └── migration-guide.md # From AWS SDK
│   │
│   ├── guides/                # How-to guides
│   │   ├── lambda-deployment.md
│   │   ├── multi-account.md
│   │   ├── testing.md
│   │   ├── performance-tuning.md
│   │   └── troubleshooting.md
│   │
│   ├── reference/             # API reference
│   │   ├── api.md
│   │   ├── struct-tags.md
│   │   ├── configuration.md
│   │   └── errors.md
│   │
│   ├── architecture/          # Design docs
│   │   ├── design.md
│   │   ├── comparison.md
│   │   ├── roadmap.md
│   │   └── internals.md
│   │
│   ├── development/           # For contributors
│   │   ├── session-summaries/ # Historical sessions
│   │   │   ├── session1/
│   │   │   ├── session2/
│   │   │   └── session3/
│   │   ├── team-prompts/      # Team coordination
│   │   └── future-work.md
│   │
│   └── pay-theory/            # Pay Theory specific
│       ├── optimizations.md
│       ├── lambda-guide.md
│       └── deployment.md
│
├── examples/                   # Example applications
│   ├── basic/                 # Simple CRUD
│   ├── lambda/                # Lambda patterns
│   ├── payment/               # Payment platform
│   ├── blog/                  # Blog application
│   ├── ecommerce/             # E-commerce
│   └── multi-tenant/          # Multi-tenant SaaS
│
├── pkg/                       # Package code
├── internal/                  # Internal packages
├── cmd/                       # Command line tools
└── tests/                     # Test suites
```

## File Mapping

### Files to Move to `docs/getting-started/`
- GETTING_STARTED.md → installation.md & quickstart.md
- Basic examples from README.md → basic-usage.md

### Files to Move to `docs/guides/`
- LAMBDA_IMPLEMENTATION_GUIDE.md → lambda-deployment.md
- TESTING_GUIDE.md → testing.md
- Multi-account content → multi-account.md

### Files to Move to `docs/reference/`
- STRUCT_TAGS.md → struct-tags.md
- API documentation from code → api.md

### Files to Move to `docs/architecture/`
- ARCHITECTURE.md → architecture.md
- DESIGN.md → design.md
- COMPARISON.md → comparison.md
- ROADMAP.md → roadmap.md

### Files to Move to `docs/development/session-summaries/`
Session 1:
- PROGRESS_SUMMARY_SESSION1.md
- TEAM1_PROMPT.md
- TEAM2_PROMPT.md
- TEAM1_SUMMARY.md
- TEAM2_TASKS.md
- TEAM2_GETTING_STARTED_SUMMARY.md
- TEAM_COORDINATION.md

Session 2:
- PROGRESS_SUMMARY_SESSION2.md
- SESSION2_IMPLEMENTATION_SUMMARY.md
- TEAM1_PROMPT_SESSION2.md
- TEAM2_PROMPT_SESSION2.md
- TEAM2_SESSION2_SUMMARY.md

Session 3:
- PROGRESS_SUMMARY_SESSION3.md
- TEAM_COORDINATION_SESSION3.md
- TEAM1_PROMPT_SESSION3.md
- TEAM2_PROMPT_SESSION3.md
- TEAM1_SESSION3_SUMMARY.md
- TEAM2_SESSION3_SUMMARY.md

### Files to Move to `docs/pay-theory/`
- PAYTHEORY_OPTIMIZATIONS.md → optimizations.md
- LAMBDA_OPTIMIZATIONS.md → lambda-guide.md
- PAYTHEORY_OPTIMIZATION_SUMMARY.md → summary.md

### Files to Archive or Merge
- PROJECT_SUMMARY.md → Merge into main README.md
- FUTURE_ENHANCEMENTS.md → docs/development/future-work.md
- Various team prompts → Consolidate in development/

### Files to Create New
- docs/README.md - Documentation index
- docs/guides/performance-tuning.md
- docs/guides/troubleshooting.md
- docs/reference/errors.md

## Implementation Steps

### Step 1: Create Directory Structure
```bash
mkdir -p docs/{getting-started,guides,reference,architecture}
mkdir -p docs/development/{session-summaries,team-prompts}
mkdir -p docs/development/session-summaries/{session1,session2,session3}
mkdir -p docs/pay-theory
```

### Step 2: Move Files (Preserve Git History)
```bash
# Use git mv to preserve history
git mv GETTING_STARTED.md docs/getting-started/quickstart.md
git mv STRUCT_TAGS.md docs/reference/struct-tags.md
# ... etc
```

### Step 3: Update References
- Update all internal links in moved files
- Update README.md with new documentation structure
- Add navigation to docs/README.md

### Step 4: Create Navigation
Create `docs/README.md`:
```markdown
# DynamORM Documentation

## Getting Started
- [Installation](getting-started/installation.md)
- [Quick Start](getting-started/quickstart.md)
- [Basic Usage](getting-started/basic-usage.md)

## Guides
- [Lambda Deployment](guides/lambda-deployment.md)
- [Multi-Account Setup](guides/multi-account.md)
...
```

### Step 5: Clean Up Root
After moving files, root should only contain:
- Essential files (README, LICENSE, etc.)
- Go source files
- Configuration files
- Directory structure

## Benefits
1. **Easier Navigation** - Clear hierarchy
2. **Better First Impression** - Clean root directory
3. **Scalable** - Easy to add new docs
4. **Separation of Concerns** - User docs vs dev docs
5. **Version Control** - Can version docs separately

## Timeline
- 1 hour: Create structure and move files
- 1 hour: Update all references
- 30 min: Create navigation files
- 30 min: Test and verify

Total: ~3 hours of work

This cleanup will make DynamORM more professional and easier to use! 