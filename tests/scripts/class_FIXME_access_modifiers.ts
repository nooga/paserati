// FIXME: Access modifiers not yet supported
// expect: "Secret: 42"
// Test access modifiers

class SecretKeeper {
    public name;          // FIXME: public modifier
    private secret;       // FIXME: private modifier  
    protected level;      // FIXME: protected modifier
    readonly id;          // FIXME: readonly modifier
    
    constructor(name, secret) {
        this.name = name;
        this.secret = secret;
        this.level = 1;
        this.id = 42;
    }
    
    public getName() {    // FIXME: public method
        return this.name;
    }
    
    private getSecret() { // FIXME: private method
        return this.secret;
    }
    
    public reveal() {     // FIXME: public method calling private
        return `Secret: ${this.getSecret()}`;
    }
}

let keeper = new SecretKeeper("Guardian", 42);
keeper.reveal();