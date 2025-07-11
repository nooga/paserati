// expect: satisfies vs as test

// Test the difference between satisfies and as

interface Animal {
    name: string;
}

function test() {
    // Object with extra properties stored in a variable first
    let petData = {
        name: "Fluffy",
        breed: "Persian",
        age: 3
    };
    
    // Using satisfies on the variable - this works because it's not an object literal
    let petSatisfies = petData satisfies Animal;
    
    // Using as - type assertion changes type to just Animal
    let petAs = petData as Animal;
    
    // With satisfies on a variable, we can still access the extra properties
    // because satisfies preserves the original type
    let breed1 = petSatisfies.breed;  // Should work
    let age1 = petSatisfies.age;      // Should work
    
    // With as, the type is changed to Animal, but runtime properties still exist
    
    if (petSatisfies.name !== "Fluffy") return "satisfies name failed";
    if (breed1 !== "Persian") return "satisfies breed failed";
    if (age1 !== 3) return "satisfies age failed";
    
    if (petAs.name !== "Fluffy") return "as name failed";
    
    // Test that object literals are stricter with satisfies
    let validPet = {
        name: "Max"
    } satisfies Animal;  // This should work - no excess properties
    
    if (validPet.name !== "Max") return "valid pet failed";
    
    return "all comparisons passed";
}

test();

"satisfies vs as test";