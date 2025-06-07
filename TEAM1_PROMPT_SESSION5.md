# Team 1 Prompt - Session 5: Documentation Restructure

## Context
You are Team 1 working on DynamORM. Session 4 successfully implemented Lambda optimizations. Now in Session 5, your mission is to transform the cluttered repository (42 .md files in root!) into a professional, well-organized open source project.

## Your Mission
Transform documentation chaos into clarity:
1. Create organized directory structure
2. Move and categorize all documentation
3. Update all internal links
4. Create navigation and index files
5. Write missing guides

## Current State Analysis
```
Root directory: 42 .md files 😱
Categories mixed: design docs, summaries, guides, prompts
Navigation: Impossible for new users
First impression: Unprofessional
```

## Target Directory Structure

```
dynamorm/
├── README.md                    # Enhanced main readme (keep in root)
├── LICENSE                      # Apache 2.0 (create in root)
├── CONTRIBUTING.md              # Contribution guide (create in root)
├── CODE_OF_CONDUCT.md          # Community standards (create in root)
├── CHANGELOG.md                # Version history (create in root)
│
├── docs/                       # All documentation
│   ├── README.md              # Documentation index
│   │
│   ├── getting-started/       # New user guides
│   │   ├── installation.md    # How to install
│   │   ├── quickstart.md      # 5-minute guide
│   │   ├── basic-usage.md     # Core concepts
│   │   └── migration-guide.md # From AWS SDK
│   │
│   ├── guides/                # How-to guides
│   │   ├── lambda-deployment.md
│   │   ├── multi-account.md
│   │   ├── testing.md
│   │   ├── performance-tuning.md
│   │   ├── troubleshooting.md
│   │   └── best-practices.md
│   │
│   ├── reference/             # API reference
│   │   ├── api.md            # Complete API docs
│   │   ├── struct-tags.md    # Tag reference
│   │   ├── configuration.md  # Config options
│   │   ├── errors.md         # Error reference
│   │   └── changelog.md      # Detailed changes
│   │
│   ├── architecture/          # Design docs
│   │   ├── overview.md       # High-level design
│   │   ├── internals.md      # Implementation details
│   │   ├── comparison.md     # vs other ORMs
│   │   └── decisions.md      # ADRs
│   │
│   ├── development/           # For contributors
│   │   ├── setup.md          # Dev environment
│   │   ├── contributing.md   # How to contribute
│   │   ├── testing.md        # Test guide
│   │   ├── releasing.md      # Release process
│   │   └── session-history/  # Historical docs
│   │
│   └── pay-theory/           # Customer-specific
│       ├── overview.md       # Pay Theory features
│       ├── optimizations.md  # Specific optimizations
│       └── deployment.md     # Deployment guide
```

## File Movement Plan

### Phase 1: Create Structure
```bash
# Create all directories
mkdir -p docs/{getting-started,guides,reference,architecture,development,pay-theory}
mkdir -p docs/development/session-history/{session1,session2,session3,session4}
```

### Phase 2: Move Files (Preserve Git History!)

#### Getting Started (Split/Merge these files)
- `GETTING_STARTED.md` → Split into:
  - `docs/getting-started/installation.md`
  - `docs/getting-started/quickstart.md`
  - `docs/getting-started/basic-usage.md`

#### Guides
- `LAMBDA_IMPLEMENTATION_GUIDE.md` → `docs/guides/lambda-deployment.md`
- `TESTING_GUIDE.md` → `docs/guides/testing.md`
- Create new: `docs/guides/multi-account.md` (from multiaccount.go)
- Create new: `docs/guides/performance-tuning.md`
- Create new: `docs/guides/troubleshooting.md`

#### Reference
- `STRUCT_TAGS.md` → `docs/reference/struct-tags.md`
- Extract API docs from code → `docs/reference/api.md`
- Create new: `docs/reference/configuration.md`
- Create new: `docs/reference/errors.md`

#### Architecture
- `ARCHITECTURE.md` → `docs/architecture/overview.md`
- `DESIGN.md` → `docs/architecture/internals.md`
- `COMPARISON.md` → `docs/architecture/comparison.md`
- `ROADMAP.md` → `docs/architecture/roadmap.md`

#### Session History (Archive these)
Move all session-related files to `docs/development/session-history/`:
- Session 1: All TEAM*_PROMPT.md, TEAM*_SUMMARY.md, PROGRESS_SUMMARY_SESSION1.md
- Session 2: All SESSION2_*.md files
- Session 3: All SESSION3_*.md files
- Session 4: All SESSION4_*.md files

#### Pay Theory Specific
- `PAYTHEORY_OPTIMIZATIONS.md` → `docs/pay-theory/optimizations.md`
- `LAMBDA_OPTIMIZATIONS.md` → `docs/pay-theory/lambda-guide.md`
- `PAYTHEORY_OPTIMIZATION_SUMMARY.md` → `docs/pay-theory/overview.md`

### Phase 3: Create New Essential Files

#### 1. Enhanced README.md (Root)
```markdown
# DynamORM - Type-Safe DynamoDB ORM for Go

[![Go Version](https://img.shields.io/badge/go-1.21+-blue.svg)]()
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)]()
[![Documentation](https://img.shields.io/badge/docs-latest-green.svg)]()

DynamORM is a Lambda-native, type-safe ORM for Amazon DynamoDB written in Go. 
It dramatically simplifies DynamoDB operations while maintaining the performance 
and scalability benefits of DynamoDB.

## ✨ Key Features
- 🚀 **Lambda-Native**: 11ms cold starts (91% faster)
- 🔒 **Type-Safe**: Full Go type safety
- 🎯 **Simple API**: 80% less code than AWS SDK
- ⚡ **High Performance**: 20,000+ ops/sec
- 🌍 **Multi-Account**: Built-in cross-account support
- 💰 **Cost Efficient**: Smart query optimization

## 🚀 Quick Start
[Include 5-line example here]

## 📚 Documentation
- [Getting Started](docs/getting-started/quickstart.md)
- [API Reference](docs/reference/api.md)
- [Examples](examples/)
- [Lambda Guide](docs/guides/lambda-deployment.md)

...
```

#### 2. LICENSE (Root)
```
Apache License 2.0
[Full Apache 2.0 text]
```

#### 3. CONTRIBUTING.md (Root)
```markdown
# Contributing to DynamORM

We love contributions! Please read our guidelines...

## Code of Conduct
[Link to CODE_OF_CONDUCT.md]

## How to Contribute
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a PR

...
```

#### 4. docs/README.md (Documentation Index)
```markdown
# DynamORM Documentation

Welcome to the DynamORM documentation!

## 📚 Documentation Structure

### For Users
- [Getting Started](getting-started/) - New to DynamORM? Start here!
- [Guides](guides/) - How-to guides for common tasks
- [Reference](reference/) - Complete API reference
- [Examples](../examples/) - Real-world examples

### For Contributors
- [Development](development/) - Contributing guide
- [Architecture](architecture/) - Design decisions

### For Pay Theory
- [Pay Theory Guide](pay-theory/) - Specific optimizations

## 🔍 Quick Links
- [5-Minute Quickstart](getting-started/quickstart.md)
- [Lambda Deployment](guides/lambda-deployment.md)
- [API Reference](reference/api.md)
...
```

### Phase 4: Update All Links

After moving files, search and replace all internal links:
- `../STRUCT_TAGS.md` → `../reference/struct-tags.md`
- Update relative paths based on new locations
- Ensure all links work

### Phase 5: Clean Up Root

Files to keep in root:
1. `README.md` (enhanced)
2. `LICENSE`
3. `CONTRIBUTING.md`
4. `CODE_OF_CONDUCT.md`
5. `CHANGELOG.md`
6. `.gitignore`
7. `go.mod`, `go.sum`
8. `Makefile`
9. Source files (*.go)
10. Config files (docker-compose.yml)

Everything else → Move to docs/

## Deliverables Checklist

### Structure
- [ ] All directories created
- [ ] All files moved with `git mv`
- [ ] Git history preserved
- [ ] No broken links

### New Documentation
- [ ] Enhanced README.md
- [ ] LICENSE file
- [ ] CONTRIBUTING.md
- [ ] CODE_OF_CONDUCT.md
- [ ] docs/README.md (index)
- [ ] Migration guide from AWS SDK
- [ ] Multi-account guide
- [ ] Performance tuning guide
- [ ] Troubleshooting guide

### Navigation
- [ ] Clear hierarchy
- [ ] Logical organization
- [ ] Easy to find content
- [ ] Good first impression

### Quality
- [ ] Professional appearance
- [ ] Consistent formatting
- [ ] Clear writing
- [ ] Helpful examples

## Success Criteria
- Root directory has ≤ 10 non-source files
- Documentation is easily navigable
- New users can get started in 5 minutes
- Contributors know how to help
- Pay Theory guides are separate but accessible

## Tools You Can Use
- `git mv` - Preserve history when moving
- `grep -r` - Find all references to update
- `tree` - Visualize directory structure
- Markdown linters - Ensure quality

## Important Notes
1. **Always use `git mv`** to preserve file history
2. **Test all links** after moving files
3. **Keep user perspective** - make it easy to find things
4. **Update Team 2** on any changes that affect examples

Remember: First impressions matter! A clean, organized repo attracts contributors and builds trust. 