import globals from "globals";
import tseslint from "typescript-eslint";

const correctness = {
  "no-cond-assign": "error",
  "no-dupe-keys": "error",
  "no-self-assign": "error",
  "no-unreachable": "error",
  "use-isnan": "error",
  "valid-typeof": "error",
};

export default [
  {
    linterOptions: { reportUnusedDisableDirectives: "off" },
  },
  {
    files: ["**/*.{js,jsx,mjs,cjs}"],
    languageOptions: {
      ecmaVersion: "latest",
      sourceType: "module",
      parserOptions: { ecmaFeatures: { jsx: true } },
      globals: { ...globals.browser, ...globals.node },
    },
    rules: { ...correctness, "no-undef": "error", "no-unused-vars": "error" },
  },
  {
    files: ["**/*.cjs"],
    languageOptions: { sourceType: "commonjs" },
  },
  {
    files: ["**/*.{ts,tsx,mts,cts}"],
    languageOptions: {
      parser: tseslint.parser,
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    plugins: { "@typescript-eslint": tseslint.plugin },
    rules: {
      ...correctness,
      "@typescript-eslint/no-duplicate-enum-values": "error",
      "@typescript-eslint/no-misused-new": "error",
      "@typescript-eslint/no-non-null-asserted-optional-chain": "error",
      "@typescript-eslint/no-unsafe-declaration-merging": "error",
      "@typescript-eslint/no-unused-vars": "error",
    },
  },
];
