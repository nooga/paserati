// Helper for module_import_defer.ts (no expectation of its own).
globalThis.__deferLog = (globalThis.__deferLog || "") + "ran;";
export const value = 42;
