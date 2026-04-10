#!/usr/bin/env bash
#
# Release Preparation Script for PARSH
#
# This script prepares a new release by:
# 1. Checking for uncommitted changes
# 2. Optionally archiving completed OpenSpec changes
# 3. Updating VERSION and CHANGELOG.md
# 4. Automatically updating Nix flake vendorHash if Go dependencies changed
# 5. Verifying the build with the new version (local test)
# 6. Creating a git commit and tag
# 7. Optionally pushing to remote
#
# IMPORTANT: Binary builds are automated via GoReleaser + GitHub Actions
# -------------------------------------------------------------------
# When you push the tag created by this script, GitHub Actions will
# automatically trigger GoReleaser, which will:
#   - Build binaries for all platforms (Linux, macOS, Windows - amd64/arm64)
#   - Create release archives (tar.gz for Unix, zip for Windows)
#   - Generate checksums
#   - Create a GitHub Release with all artifacts
#
# The local build in this script (step 5) is only for verification.
# The official release binaries are built by GoReleaser in the cloud.
#
# Nix vendorHash Automation (step 4):
# -----------------------------------
# When Go module dependencies (go.mod/go.sum) change between releases,
# the Nix flake's vendorHash must be updated. This script automatically:
#   - Detects dependency changes by comparing with the last release tag
#   - Calculates the correct hash using `nix build` (parses error output)
#   - Updates flake.nix with the new hash
#   - Includes flake.nix in the release commit
# If Nix is unavailable or hash calculation fails, a warning is shown
# and the release continues (allowing manual hash update if needed).
#
# Workflow:
#   ./release.sh  →  Creates tag  →  git push origin v*.*.*
#                                        ↓
#                                  GitHub Actions + GoReleaser
#                                        ↓
#                                  Multi-platform binaries published
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    print_error "Not in a git repository"
    exit 1
fi

# Check for uncommitted changes
if [[ -n $(git status --porcelain) ]]; then
    print_error "You have uncommitted changes. Please commit or stash them first."
    git status --short
    exit 1
fi

# Check remote connectivity
print_info "Checking remote connectivity..."
if ! git ls-remote --exit-code origin &>/dev/null; then
    print_error "Cannot reach remote repository. Check your network connection."
    exit 1
fi
print_success "Remote is reachable"

# Read current version from VERSION
if [[ ! -f VERSION ]]; then
    print_error "VERSION file not found"
    exit 1
fi

CURRENT_VERSION=$(cat VERSION | tr -d '[:space:]')
print_info "Current version: ${GREEN}${CURRENT_VERSION}${NC}"

# Parse version components
IFS='.' read -r -a VERSION_PARTS <<< "$CURRENT_VERSION"
MAJOR="${VERSION_PARTS[0]}"
MINOR="${VERSION_PARTS[1]}"
PATCH="${VERSION_PARTS[2]}"

echo ""
echo "Select release type:"
echo "  1) Patch   (${MAJOR}.${MINOR}.$((PATCH + 1))) - Bug fixes, no new features"
echo "  2) Minor   (${MAJOR}.$((MINOR + 1)).0) - New features, backwards compatible"
echo "  3) Major   ($((MAJOR + 1)).0.0) - Breaking changes"
echo ""
read -p "Enter choice (1-3): " RELEASE_TYPE

case $RELEASE_TYPE in
    1)
        RELEASE_NAME="patch"
        NEW_VERSION="${MAJOR}.${MINOR}.$((PATCH + 1))"
        ;;
    2)
        RELEASE_NAME="minor"
        NEW_VERSION="${MAJOR}.$((MINOR + 1)).0"
        ;;
    3)
        RELEASE_NAME="major"
        NEW_VERSION="$((MAJOR + 1)).0.0"
        ;;
    *)
        print_error "Invalid choice"
        exit 1
        ;;
esac

print_info "New version will be: ${GREEN}${NEW_VERSION}${NC} (${RELEASE_NAME} release)"

# Check if tag already exists
if git rev-parse "v${NEW_VERSION}" >/dev/null 2>&1; then
    print_error "Tag v${NEW_VERSION} already exists!"
    print_info "Existing tags:"
    git tag -l | grep -E "v${MAJOR}\.${MINOR}\." | tail -5
    exit 1
fi
print_success "Version tag is available"

# Check for OpenSpec completed changes that need archiving
if command -v openspec &> /dev/null; then
    print_info "Checking OpenSpec changes..."

    COMPLETED_CHANGES=$(openspec list 2>/dev/null | grep "✓ Complete" | awk '{print $1}' || true)

    if [[ -n "$COMPLETED_CHANGES" ]]; then
        print_warning "Found completed OpenSpec changes that may need archiving:"
        echo ""
        openspec list | grep "✓ Complete" || true
        echo ""
        read -p "Archive these changes before release? (y/n): " ARCHIVE_CHANGES

        if [[ $ARCHIVE_CHANGES == "y" || $ARCHIVE_CHANGES == "Y" ]]; then
            print_info "Archiving completed changes..."
            for change in $COMPLETED_CHANGES; do
                print_info "Archiving ${change}..."
                if openspec archive "$change" --yes 2>/dev/null; then
                    print_success "Archived ${change}"
                else
                    print_warning "Could not archive ${change} (may already be archived)"
                fi
            done

            # Commit archived changes if any
            if [[ -n $(git status --porcelain openspec/) ]]; then
                print_info "Committing archived changes..."
                git add openspec/
                git commit -m "chore: archive completed openspec changes before release"
                print_success "Archived changes committed"
            fi
        else
            print_warning "Skipping OpenSpec archiving"
        fi
    else
        print_success "No completed OpenSpec changes to archive"
    fi
else
    print_warning "OpenSpec CLI not found, skipping OpenSpec checks"
fi
echo ""

# Ask for changelog entry
print_info "CHANGELOG.md update"
echo ""
echo "Choose changelog option:"
echo "  1) I have already added entries under '## NEXT VERSION' in CHANGELOG.md"
echo "  2) Enter changelog entries now (interactive)"
echo ""
read -p "Enter choice (1-2): " CHANGELOG_CHOICE

if [[ $CHANGELOG_CHOICE == "2" ]]; then
    echo ""
    print_info "Enter changelog entries (one per line, press Ctrl+D when done):"
    print_info "Format: '- Feature: description' or '- Fix: description' or '- Enhancement: description'"
    echo ""

    CHANGELOG_ENTRIES=""
    while IFS= read -r line; do
        if [[ -n "$line" ]]; then
            CHANGELOG_ENTRIES="${CHANGELOG_ENTRIES}${line}\n"
        fi
    done

    if [[ -z "$CHANGELOG_ENTRIES" ]]; then
        print_error "No changelog entries provided"
        exit 1
    fi
fi

# Get current date
RELEASE_DATE=$(date +"%d %b %Y" | sed 's/ 0/ /g')

# Update CHANGELOG.md
print_info "Updating CHANGELOG.md..."

if [[ $CHANGELOG_CHOICE == "1" ]]; then
    # Replace "## NEXT VERSION" with actual version and date
    if grep -q "## NEXT VERSION" CHANGELOG.md; then
        sed -i "s/## NEXT VERSION/## [${NEW_VERSION}] - ${RELEASE_DATE}/" CHANGELOG.md
        print_success "Updated CHANGELOG.md (replaced NEXT VERSION)"
    else
        # No NEXT VERSION found, add new section at top
        print_warning "No '## NEXT VERSION' found in CHANGELOG.md"
        echo "Please add changelog entries manually before continuing."
        exit 1
    fi
else
    # Insert new version section with provided entries
    TEMP_FILE=$(mktemp)
    {
        head -n 6 CHANGELOG.md
        echo ""
        echo "## [${NEW_VERSION}] - ${RELEASE_DATE}"
        echo -e "$CHANGELOG_ENTRIES"
        echo ""
        tail -n +7 CHANGELOG.md
    } > "$TEMP_FILE"
    mv "$TEMP_FILE" CHANGELOG.md
    print_success "Updated CHANGELOG.md with new entries"
fi

# Update VERSION
print_info "Updating VERSION..."
echo "$NEW_VERSION" > VERSION
print_success "Updated VERSION to ${NEW_VERSION}"

# Automatic vendorHash update for Nix flake (if Go dependencies changed)
# This section detects if go.mod/go.sum changed since the last release tag,
# calculates the correct vendorHash using Nix, and updates flake.nix automatically.
# If Nix is unavailable or calculation fails, a warning is shown but release continues.
echo ""
if [[ -f flake.nix ]] && [[ -f go.mod ]]; then
    print_info "Checking if Go dependencies changed since last release..."

    # Get last release tag
    LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")

    # Check if go.mod or go.sum changed
    DEPS_CHANGED=false
    if [[ -z "$LAST_TAG" ]]; then
        print_info "No previous release tag found (first release)"
        DEPS_CHANGED=true
    elif git diff "${LAST_TAG}..HEAD" -- go.mod go.sum | grep -q .; then
        print_info "Go dependencies changed since ${LAST_TAG}"
        DEPS_CHANGED=true
    else
        print_success "No Go dependency changes detected"
    fi

    # Update vendorHash if dependencies changed
    if [[ "$DEPS_CHANGED" == "true" ]]; then
        if command -v nix &> /dev/null; then
            print_info "Calculating correct vendorHash..."

            # Create backup of flake.nix
            cp flake.nix flake.nix.backup

            # Temporarily set vendorHash to empty string to trigger hash calculation
            sed -i 's|vendorHash = "sha256-[^"]*";|vendorHash = "";|' flake.nix

            # Try to build and capture the correct hash from error output
            VENDOR_HASH=""
            BUILD_OUTPUT=$(nix build .#default 2>&1 || true)

            # Parse hash from output (Nix shows: "got: sha256-...")
            VENDOR_HASH=$(echo "$BUILD_OUTPUT" | grep -oP 'got:\s+\Ksha256-[A-Za-z0-9+/=]+' | head -1)

            # Restore backup
            mv flake.nix.backup flake.nix

            if [[ -n "$VENDOR_HASH" ]] && [[ "$VENDOR_HASH" =~ ^sha256-[A-Za-z0-9+/=]+$ ]]; then
                print_info "Calculated vendorHash: ${VENDOR_HASH}"

                # Update flake.nix with new hash
                sed -i "s|vendorHash = \"sha256-[^\"]*\";|vendorHash = \"${VENDOR_HASH}\";|" flake.nix

                # Verify the update worked
                if grep -q "vendorHash = \"${VENDOR_HASH}\"" flake.nix; then
                    print_success "Updated vendorHash in flake.nix"

                    # Stage flake.nix for commit
                    git add flake.nix
                    print_info "flake.nix will be included in release commit"
                else
                    print_error "Failed to update vendorHash in flake.nix"
                    print_warning "You may need to update it manually after release"
                fi
            else
                print_warning "Could not calculate vendorHash automatically"
                print_info "Manual update: Run 'nix build .#parsh 2>&1 | grep got:' and update flake.nix"
            fi
        else
            print_warning "Nix not found - skipping automatic vendorHash update"
            print_info "Install Nix for automated vendorHash updates, or update manually:"
            print_info "  nix build .#parsh 2>&1 | grep 'got:'"
        fi
    fi
fi
echo ""

# Verify Nix flake if available
if [[ -f flake.nix ]] && command -v nix &> /dev/null; then
    print_info "Verifying Nix flake..."

    # Check flake
    if nix flake check 2>&1 | grep -q "error:"; then
        print_warning "Nix flake check found issues (this may be normal)"
    else
        print_success "Nix flake check passed"
    fi

    # Optionally update flake.lock
    echo ""
    read -p "Update flake.lock (nix flake update)? (y/n): " UPDATE_FLAKE

    if [[ $UPDATE_FLAKE == "y" || $UPDATE_FLAKE == "Y" ]]; then
        print_info "Updating flake.lock..."
        nix flake update
        print_success "flake.lock updated"

        if [[ -n $(git status --porcelain flake.lock) ]]; then
            git add flake.lock
            print_info "flake.lock will be included in release commit"
        fi
    fi
else
    if [[ ! -f flake.nix ]]; then
        print_warning "flake.nix not found, skipping Nix checks"
    else
        print_warning "Nix not installed, skipping flake checks"
    fi
fi

# Verify Go build with version
# NOTE: This is a local verification build only.
# The official multi-platform binaries will be built by GoReleaser
# on GitHub Actions when you push the tag.
if [[ -f go.mod ]] && command -v go &> /dev/null; then
    print_info "Verifying Go build with version ${NEW_VERSION}..."
    print_info "(Official binaries will be built by GoReleaser after tag push)"

    # Build with version ldflags
    if go build -ldflags "-X main.Version=${NEW_VERSION}" -o parsh.test 2>&1; then
        print_success "Go build succeeded"

        # Test version output
        VERSION_OUTPUT=$(./parsh.test --version 2>&1 || echo "")
        if echo "$VERSION_OUTPUT" | grep -q "${NEW_VERSION}"; then
            print_success "Version ${NEW_VERSION} embedded correctly"
        else
            print_warning "Version check: output was '${VERSION_OUTPUT}'"
        fi

        # Clean up test binary
        rm -f parsh.test
    else
        print_error "Go build failed!"
        print_error "Fix build errors before releasing"
        exit 1
    fi

    # Check for go.mod/go.sum changes
    if [[ -n $(git status --porcelain go.mod go.sum 2>/dev/null) ]]; then
        print_warning "go.mod or go.sum has uncommitted changes"
        print_info "Run 'go mod tidy' if dependencies changed"

        read -p "Continue anyway? (y/n): " CONTINUE_BUILD
        if [[ $CONTINUE_BUILD != "y" && $CONTINUE_BUILD != "Y" ]]; then
            print_error "Aborting release"
            exit 1
        fi
    fi
else
    if [[ ! -f go.mod ]]; then
        print_warning "go.mod not found, skipping Go build checks"
    else
        print_warning "Go not installed, skipping build checks"
    fi
fi
echo ""

# Show summary
echo ""
echo "════════════════════════════════════════════════════════════"
print_info "Release Summary"
echo "════════════════════════════════════════════════════════════"
echo ""
echo "  Release type:    ${RELEASE_NAME}"
echo "  Old version:     ${CURRENT_VERSION}"
echo "  New version:     ${GREEN}${NEW_VERSION}${NC}"
echo "  Release date:    ${RELEASE_DATE}"
echo ""
echo "  Files changed:"
echo "    - CHANGELOG.md"
echo "    - VERSION"
if [[ -n $(git status --porcelain flake.nix 2>/dev/null) ]]; then
    echo "    - flake.nix (vendorHash updated)"
fi
if [[ -n $(git status --porcelain flake.lock 2>/dev/null) ]]; then
    echo "    - flake.lock"
fi
if [[ -n $(git status --porcelain openspec/ 2>/dev/null) ]]; then
    echo "    - openspec/ (archived changes)"
fi
echo ""
echo "════════════════════════════════════════════════════════════"
echo ""

# Show git diff
print_info "Changes to be committed:"
echo ""
git diff HEAD CHANGELOG.md VERSION flake.nix flake.lock 2>/dev/null || git diff CHANGELOG.md VERSION
if [[ -n $(git status --porcelain openspec/ 2>/dev/null) ]]; then
    echo ""
    print_info "OpenSpec changes:"
    git status --short openspec/
fi
echo ""

# Confirm before committing
read -p "Commit these changes? (y/n): " CONFIRM_COMMIT

if [[ $CONFIRM_COMMIT != "y" && $CONFIRM_COMMIT != "Y" ]]; then
    print_warning "Aborting release. Rolling back changes..."
    git checkout CHANGELOG.md VERSION 2>/dev/null || true
    git checkout flake.nix 2>/dev/null || true
    git checkout flake.lock 2>/dev/null || true
    print_info "Changes rolled back"
    exit 0
fi

# Create git commit
print_info "Creating git commit..."
git add CHANGELOG.md VERSION

# Add flake.nix if it was updated (vendorHash)
if [[ -n $(git status --porcelain flake.nix 2>/dev/null) ]]; then
    git add flake.nix
fi

# Add flake.lock if it was updated
if [[ -n $(git status --porcelain flake.lock 2>/dev/null) ]]; then
    git add flake.lock
fi

# Build commit message
COMMIT_MESSAGE="release: bump version to ${NEW_VERSION}

Update VERSION and CHANGELOG.md for ${RELEASE_NAME} release."

# Add note about flake.nix if it was updated
if [[ -n $(git status --porcelain --cached flake.nix 2>/dev/null) ]]; then
    COMMIT_MESSAGE="${COMMIT_MESSAGE}
Update flake.nix vendorHash for Go dependency changes."
fi

git commit -m "$COMMIT_MESSAGE"
print_success "Commit created"

# Create git tag
print_info "Creating git tag v${NEW_VERSION}..."

TAG_MESSAGE="Release v${NEW_VERSION}

$(sed -n "/## \[${NEW_VERSION}\]/,/## \[[0-9]/p" CHANGELOG.md | sed '$d' | tail -n +2)"

git tag -a "v${NEW_VERSION}" -m "$TAG_MESSAGE"
print_success "Tag v${NEW_VERSION} created"

# Show final summary
echo ""
echo "════════════════════════════════════════════════════════════"
print_success "Release v${NEW_VERSION} prepared successfully!"
echo "════════════════════════════════════════════════════════════"
echo ""
echo "  Commit: $(git log -1 --pretty=format:'%h - %s')"
echo "  Tag:    v${NEW_VERSION}"
echo ""
echo "════════════════════════════════════════════════════════════"
echo ""

# Confirm before pushing
read -p "Push to remote (commit + tag)? (y/n): " CONFIRM_PUSH

if [[ $CONFIRM_PUSH != "y" && $CONFIRM_PUSH != "Y" ]]; then
    print_warning "Skipping push. You can push later with:"
    echo ""
    echo "  git push origin main"
    echo "  git push origin v${NEW_VERSION}"
    echo ""
    exit 0
fi

# Push to remote
print_info "Pushing to remote..."
git push origin main
git push origin "v${NEW_VERSION}"

echo ""
print_success "Release v${NEW_VERSION} pushed to remote!"
echo ""
print_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
print_info "  GitHub Actions will now automatically:"
echo "    • Build binaries for all platforms (Linux, macOS, Windows)"
echo "    • Create release archives (tar.gz, zip)"
echo "    • Generate checksums"
echo "    • Publish GitHub Release with all artifacts"
echo ""
print_info "  Monitor the build:"
echo "    https://github.com/torreirow/parsh/actions"
echo ""
print_info "  View the release (when complete):"
echo "    https://github.com/torreirow/parsh/releases/tag/v${NEW_VERSION}"
print_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
print_success "Done! 🚀"
echo ""
