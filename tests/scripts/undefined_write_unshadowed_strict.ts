// skip-typecheck
// expect_runtime_error: Cannot assign to read only property 'undefined'
// paserati#440 follow-up: same as undefined_write_unshadowed_sloppy.ts, but
// in strict mode assignment to the non-writable global `undefined` throws
// TypeError instead of silently no-op'ing (real Node: TypeError: Cannot
// assign to read only property 'undefined' of object '#<Object>').
"use strict";
undefined = 43;
undefined;
