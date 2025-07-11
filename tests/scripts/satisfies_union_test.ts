// expect: satisfies union test

// Test satisfies with union types

type StringOrNumber = string | number;

function test() {
    // These should all work - each value satisfies StringOrNumber
    let str = "hello" satisfies StringOrNumber;
    let num = 42 satisfies StringOrNumber;
    
    // The types should be preserved as their specific literal types
    if (str !== "hello") return "string test failed";
    if (num !== 42) return "number test failed";
    
    // Test with more complex union
    type Config = {
        apiUrl: string;
        timeout: number;
    } | {
        apiUrl: string;
        retries: number;
    };
    
    let config1 = {
        apiUrl: "https://api.example.com",
        timeout: 5000
    } satisfies Config;
    
    let config2 = {
        apiUrl: "https://api.example.com", 
        retries: 3
    } satisfies Config;
    
    if (config1.apiUrl !== "https://api.example.com") return "config1 apiUrl failed";
    if (config1.timeout !== 5000) return "config1 timeout failed";
    
    if (config2.apiUrl !== "https://api.example.com") return "config2 apiUrl failed";
    if (config2.retries !== 3) return "config2 retries failed";
    
    return "all union tests passed";
}

test();

"satisfies union test";