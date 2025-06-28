// expect: Testing override with inheritance syntax
// Test override keyword with inheritance syntax (inheritance not yet implemented)

class Animal {
    name: string;
    
    constructor(name: string) {
        this.name = name;
    }
    
    speak(): string {
        return "Generic sound";
    }
}

class Dog extends Animal {
    breed: string;
    
    constructor(name: string, breed: string) {
        super(name);
        this.breed = breed;
    }
    
    // This should not generate an override error since Dog extends Animal
    override speak(): string {
        return "Woof!";
    }
}

"Testing override with inheritance syntax";