// FIXME: Class inheritance not yet supported
// expect: "Dog named Buddy says Woof!"
// Test class inheritance

class Animal {
    name;
    
    constructor(name) {
        this.name = name;
    }
    
    speak() {
        return "Generic sound";
    }
    
    getName() {
        return this.name;
    }
}

class Dog extends Animal {  // FIXME: extends keyword
    breed;
    
    constructor(name, breed) {
        super(name);        // FIXME: super() call
        this.breed = breed;
    }
    
    speak() {              // FIXME: method override
        return "Woof!";
    }
    
    describe() {
        return `Dog named ${super.getName()} says ${this.speak()}`;  // FIXME: super.method()
    }
}

let dog = new Dog("Buddy", "Labrador");
dog.describe();