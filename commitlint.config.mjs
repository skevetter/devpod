export default {
  extends: ["@commitlint/config-conventional"],
  rules: {
    "type-enum": [
      2,
      "always",
      [
        "build",
        "ci",
        "docs",
        "feat",
        "fix",
        "perf",
        "refactor",
        "style",
        "test",
        "chore",
        "revert",
        "bump",
        "fixup",
      ],
    ],
    "body-max-line-length": [0, "always", Infinity],
  },
}
