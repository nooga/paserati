// expect: application/json
// Bug 4: index operator on Record<string, string> and index signature types
const headers: Record<string, string> = { "content-type": "application/json" };
const ct = headers["content-type"];
ct;
