// expect: enum in class test

// Test enum usage with classes

enum Status {
    Active,   // 0
    Inactive  // 1
}

class User {
    private status: Status;
    
    constructor(public name: string, status: Status = Status.Active) {
        this.status = status;
    }
    
    getStatus(): Status {
        return this.status;
    }
    
    setStatus(status: Status): void {
        this.status = status;
    }
    
    isActive(): boolean {
        return this.status === Status.Active;
    }
}

function test() {
    // Test default enum value
    let user1 = new User("Alice");
    if (!user1.isActive()) return "user1 should be active by default";
    
    // Test explicit enum value
    let user2 = new User("Bob", Status.Inactive);
    if (user2.isActive()) return "user2 should be inactive";
    
    // Test enum comparison
    if (user2.getStatus() !== Status.Inactive) return "user2 status should be Inactive";
    
    // Test enum method parameter
    user2.setStatus(Status.Active);
    if (!user2.isActive()) return "user2 should be active after setting";
    
    return "all tests passed";
}

test();

"enum in class test";