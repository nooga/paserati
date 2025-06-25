// Test complex object spread with deep merging pattern
let config = {api: {url: "localhost", port: 3000}};
let overrides = {api: {port: 8080, timeout: 5000}};
let merged = {...config, api: {...config.api, ...overrides.api}};
merged;
// expect: {api: {url: "localhost", port: 8080, timeout: 5000}}