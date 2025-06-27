// expect: BankAccount #123: $1000
// Test access modifiers (public, private, protected)

class BankAccount {
  // Public field (default)
  public accountNumber: string;

  // Private fields
  private balance: number;
  private pin: string;

  // Protected field
  protected bankName: string;

  // Readonly public field
  public readonly accountType: string = "checking";

  constructor(accountNumber: string, initialBalance: number, pin: string) {
    this.accountNumber = accountNumber;
    this.balance = initialBalance;
    this.pin = pin;
    this.bankName = "Test Bank";
  }

  // Public method
  public getBalance(): number {
    return this.balance;
  }

  // Public method using private field
  public withdraw(amount: number, enteredPin: string): boolean {
    if (this.validatePin(enteredPin) && this.balance >= amount) {
      this.balance -= amount;
      return true;
    }
    return false;
  }

  // Private method
  private validatePin(enteredPin: string): boolean {
    return this.pin === enteredPin;
  }

  // Protected method
  protected getBankInfo(): string {
    return this.bankName;
  }

  // Public method for display
  public toString(): string {
    return `BankAccount #${this.accountNumber}: $${this.balance}`;
  }
}

class SavingsAccount extends BankAccount {
  private interestRate: number;

  constructor(
    accountNumber: string,
    initialBalance: number,
    pin: string,
    rate: number
  ) {
    super(accountNumber, initialBalance, pin);
    this.interestRate = rate;
  }

  // Can access protected members from parent
  public getBankDetails(): string {
    return `Savings at ${this.getBankInfo()}`;
  }

  public addInterest(): void {
    // Cannot access private balance directly, must use public method
    let currentBalance = this.getBalance();
    // Would need a deposit method to actually add interest
  }
}

let account = new BankAccount("123", 1000, "1234");
account.toString();
