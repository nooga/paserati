// expect_runtime_error: VM PANIC RECOVERED
// Test comprehensive import patterns - all should parse successfully
import defaultExport from "module-name";
import * as name from "module-name";  
import { export1 } from "module-name";
import { export1 as alias1 } from "module-name";
import { default as alias } from "module-name";
import { export1, export2 } from "module-name";
import { export1, export2 as alias2 } from "module-name";
import { "string name" as alias } from "module-name";
import defaultExport2, { export1 as alias3 } from "module-name";
import defaultExport3, * as name2 from "module-name";
import "module-name";

// Basic code to avoid pure import file
const x = 42;
x;