// expect: ok
// declare const registers the type for the checker but produces no runtime code.
// We just verify it parses and compiles without errors.
declare const config: { apiKey: string; model: string; };
declare let counter: number;
declare var globalFlag: boolean;
"ok";
