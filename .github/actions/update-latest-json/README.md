# Update Latest JSON Action

TypeScript action to update latest.json with release information and rename assets.

## Development

```bash
# Install dependencies
npm install

# Build the action
npm run build

# Lint
npm run lint

# Format
npm run format
```

## Building

The action must be built before committing:

```bash
npm run build
git add dist/
git commit -m "Build action"
```

The `dist/` directory should be committed to the repository.
