// Test cases that might trigger semicolon parsing issues
for (let i = 0; i < 10; i++) {
    if (i === 5) continue;
    console.log(i);
}