// Mutually recursive interfaces
// expect: Alice manages Bob

// Forward reference works: Employee references Manager before it's declared
interface Employee {
  name: string;
  manager: Manager | null;
}

interface Manager {
  name: string;
  reports: Employee[];
}

const bob: Employee = { name: "Bob", manager: null };
const alice: Manager = { name: "Alice", reports: [bob] };
bob.manager = alice;

// Access through mutual references
const emp = alice.reports[0];
const mgr = emp.manager as Manager;
mgr.name + " manages " + emp.name;
