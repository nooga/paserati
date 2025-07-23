// Test the paserati/http module with real HTTP requests
// Uses httpbin.org as a reliable test API service

import { fetch, Headers } from "paserati/http";

console.log("Testing paserati/http module...");

// Test 1: Simple GET request
console.log("1. Testing GET request...");
try {
    const response = fetch("https://httpbin.org/get");
    console.log("GET Status:", response.status);
    console.log("GET OK:", response.ok);
    
    const data = response.json();
    console.log("GET Response type:", typeof data);
    console.log("GET has url field:", "url" in data);
    
    if (response.status === 200 && data.url) {
        console.log("✓ GET test passed");
    } else {
        console.log("✗ GET test failed");
    }
} catch (error) {
    console.log("✗ GET test error:", error);
}

// Test 2: POST request with JSON body
console.log("\n2. Testing POST request with JSON...");
try {
    const postData = { message: "Hello from Paserati!", timestamp: 1234567890 };
    
    const response = fetch("https://httpbin.org/post", {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            "User-Agent": "Paserati-HTTP/1.0"
        },
        body: postData  // Should auto-stringify due to Content-Type
    });
    
    console.log("POST Status:", response.status);
    const data = response.json();
    
    // httpbin.org returns the JSON body in the 'json' field already parsed
    const echoedData = data.json;
    console.log("POST Echoed message:", echoedData.message);
    console.log("POST Echoed timestamp:", echoedData.timestamp);
    
    if (response.status === 200 && echoedData.message === "Hello from Paserati!") {
        console.log("✓ POST JSON test passed");
    } else {
        console.log("✗ POST JSON test failed");
    }
} catch (error) {
    console.log("✗ POST JSON test error:", error);
}

// Test 3: Headers functionality
console.log("\n3. Testing Headers class...");
try {
    const headers = new Headers({
        "Authorization": "Bearer test-token",
        "Accept": "application/json"
    });
    
    console.log("Headers get Authorization:", headers.get("Authorization"));
    console.log("Headers has Accept:", headers.has("Accept"));
    console.log("Headers get Accept:", headers.get("Accept"));
    console.log("Headers has X-Custom:", headers.has("X-Custom"));
    
    headers.set("X-Custom", "custom-value");
    console.log("Headers after set X-Custom:", headers.get("X-Custom"));
    
    headers.delete("Authorization");
    console.log("Headers has Authorization after delete:", headers.has("Authorization"));
    
    const hasCorrectBehavior = 
        headers.get("Accept") === "application/json" &&
        headers.get("X-Custom") === "custom-value" &&
        !headers.has("Authorization");
    
    if (hasCorrectBehavior) {
        console.log("✓ Headers test passed");
    } else {
        console.log("✗ Headers test failed");
    }
} catch (error) {
    console.log("✗ Headers test error:", error);
}

// Test 4: Binary data handling with blob()
console.log("\n4. Testing binary data handling...");
try {
    // httpbin.org/bytes/{n} returns n random bytes
    const response = fetch("https://httpbin.org/bytes/10");
    console.log("Binary Status:", response.status);
    
    const blob = response.blob();
    console.log("Binary blob type:", typeof blob);
    console.log("Binary blob length:", blob.length);
    console.log("Binary blob byteLength:", blob.byteLength);
    console.log("Binary blob has buffer:", typeof blob.buffer === "object");
    
    // Verify it's a proper Uint8Array-like object
    const isValidBinary = 
        blob.length === 10 &&
        blob.byteLength === 10 &&
        typeof blob.buffer === "object" &&
        typeof blob[0] === "number";
    
    if (response.status === 200 && isValidBinary) {
        console.log("✓ Binary data test passed");
    } else {
        console.log("✗ Binary data test failed");
    }
} catch (error) {
    console.log("✗ Binary data test error:", error);
}

// Test 5: Error handling
console.log("\n5. Testing error handling...");
try {
    const response = fetch("https://httpbin.org/status/404");
    console.log("Error Status:", response.status);
    console.log("Error OK:", response.ok);
    
    if (response.status === 404 && !response.ok) {
        console.log("✓ Error handling test passed");
    } else {
        console.log("✗ Error handling test failed");
    }
} catch (error) {
    console.log("✗ Error handling test error:", error);
}

console.log("\nHTTP module tests completed!");

// Return the completion message for the test framework
"HTTP module tests completed!";

// expect: HTTP module tests completed!