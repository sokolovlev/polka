// Shared ESLint config for TS workspaces (web has its own, extending Next's).
import js from "@eslint/js";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["**/dist/**", "**/node_modules/**", "**/src/gen/**", "web/**"] },
  js.configs.recommended,
  ...tseslint.configs.strictTypeChecked,
  {
    languageOptions: {
      parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
    },
  },
  {
    files: ["**/*.mjs", "**/*.js"],
    ...tseslint.configs.disableTypeChecked,
  },
  {
    files: ["**/test/**/*.ts", "**/*.test.ts"],
    rules: { "@typescript-eslint/no-non-null-assertion": "off" },
  },
);
