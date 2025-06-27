class Person {
    private _name: string;
    private _age: number;
    
    constructor(name: string, age: number) {
        this._name = name;
        this._age = age;
    }
    
    get name(): string {
        return this._name;
    }
    
    get isAdult(): boolean {
        return this._age >= 18;
    }
    
    toString(): string {
        return this.name + " (valid: " + this.isAdult + ")";
    }
}

let person = new Person("John", 25);
person.toString();