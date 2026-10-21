// Stylesheet imports are resolved by the bundler, never by the type checker: a
// `*.css` import has no exports, it only has the side effect of loading the
// stylesheet. The harness (vite) and the test runner both handle that already;
// this declaration is what lets `tsc` accept the import instead of reporting a
// missing module.
declare module "*.css"
