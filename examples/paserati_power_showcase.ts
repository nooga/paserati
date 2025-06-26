// Paserati Power Showcase - Demonstrating Advanced TypeScript Features
// This example showcases generics, type narrowing, error handling, spread syntax,
// destructuring, functional programming, and more - all working in Paserati!

console.log("🚀 Paserati Power Showcase");
console.log("===========================");

// Generic interfaces and types
interface DataPoint<T extends string | number> {
  id: string;
  value: T;
  timestamp: number;
}

interface User {
  name: string;
  age: number;
  skills: string[];
  active: boolean;
}

// Advanced generic utility types
type Transformer<TInput, TOutput> = (input: TInput) => TOutput;
type Validator<T> = (value: T) => boolean;

// Sample datasets
const users: User[] = [
  {
    name: "Alice",
    age: 28,
    skills: ["TypeScript", "Go", "Python"],
    active: true,
  },
  { name: "Bob", age: 32, skills: ["JavaScript", "Rust"], active: false },
  {
    name: "Charlie",
    age: 24,
    skills: ["TypeScript", "C++", "WebAssembly"],
    active: true,
  },
];

const salesData: DataPoint<number>[] = [
  { id: "Q1-2024", value: 125000, timestamp: Date.now() - 86400000 },
  { id: "Q2-2024", value: 98000, timestamp: Date.now() - 43200000 },
  { id: "Q3-2024", value: 152000, timestamp: Date.now() - 21600000 },
];

// Generic data processor with method chaining
class DataProcessor<T> {
  private data: T[];

  constructor(initialData: T[]) {
    this.data = [...initialData]; // Spread syntax for immutability
  }

  // Generic transformation with type inference
  transform<U>(transformer: Transformer<T, U>): DataProcessor<U> {
    const transformed = this.data.map(transformer);
    return new DataProcessor(transformed);
  }

  filter(predicate: Validator<T>): DataProcessor<T> {
    return new DataProcessor(this.data.filter(predicate));
  }

  // Aggregation with advanced type handling
  aggregate<TResult>(
    initialValue: TResult,
    aggregator: (acc: TResult, current: T) => TResult
  ): TResult {
    return this.data.reduce(aggregator, initialValue);
  }

  getData(): T[] {
    return [...this.data]; // Defensive copying
  }

  getCount(): number {
    return this.data.length;
  }
}

// Text processing with regular expressions and type narrowing
function processText(input: unknown): string {
  // Type narrowing with typeof guards
  if (typeof input !== "string") {
    throw new TypeError(`Expected string, got ${typeof input}`);
  }

  // Regular expression processing
  const emailRegex = /[\w.-]+@[\w.-]+\.\w+/g;
  const phoneRegex = /\d{3}-\d{4}/g;

  let processed = input;

  // Replace sensitive data with masks
  processed = processed.replace(emailRegex, "[EMAIL_HIDDEN]");
  processed = processed.replace(phoneRegex, "[PHONE_HIDDEN]");

  return processed;
}

// Complex destructuring patterns
function analyzeUsers(): { results: any[]; summary: any } {
  console.log("\n📊 User Data Analysis");
  console.log("---------------------");

  // Destructuring with array patterns
  const results = users.map((user) => {
    // Destructure user object
    const { name, age, skills, active } = user;

    // Destructure skills array with rest pattern
    const [primarySkill, ...otherSkills] = skills;

    return {
      name,
      demographic: age >= 30 ? "senior" : "junior",
      expertise: {
        primary: primarySkill || "none",
        additionalCount: otherSkills.length,
        hasTypeScript: skills.includes("TypeScript"),
      },
      status: active ? "active" : "inactive",
    };
  });

  // Advanced aggregation with spread syntax
  const summary = results.reduce(
    (acc, user) => {
      return {
        ...acc, // Spread existing properties
        [user.demographic]: (acc[user.demographic] || 0) + 1,
        activeTypeScriptUsers:
          acc.activeTypeScriptUsers +
          (user.status === "active" && user.expertise.hasTypeScript ? 1 : 0),
      };
    },
    { activeTypeScriptUsers: 0 } as { [key: string]: number }
  );

  console.log("Analysis Results:", JSON.stringify(results, null, 2));
  console.log("Summary:", JSON.stringify(summary, null, 2));

  return { results, summary };
}

// Higher-order functions with generics
function compose<T, U, V>(f: (x: U) => V, g: (x: T) => U): (x: T) => V {
  return (x: T) => f(g(x));
}

// Sales data processing pipeline
function processSalesData() {
  console.log("\n💰 Sales Data Processing");
  console.log("-------------------------");

  const processor = new DataProcessor(salesData);

  // Transform with destructuring and spread
  const enriched = processor.transform((dataPoint) => {
    const { id, value, timestamp } = dataPoint;
    const [quarter, year] = id.split("-");

    return {
      ...dataPoint, // Spread original properties
      quarter,
      year: parseInt(year),
      valueInK: Math.round(value / 1000),
    };
  });

  // Filter for current year
  const current = enriched.filter((data) => data.year >= 2024);

  // Aggregate sales data
  const totals = current.aggregate(
    { totalSales: 0, quarters: [] as string[] },
    (acc, item) => ({
      totalSales: acc.totalSales + item.value,
      quarters: acc.quarters.includes(item.quarter)
        ? acc.quarters
        : [...acc.quarters, item.quarter],
    })
  );

  const avgSales = Math.round(totals.totalSales / totals.quarters.length);

  console.log(
    "Sales Summary:",
    JSON.stringify(
      {
        ...totals,
        avgSales,
        count: current.getCount(),
      },
      null,
      2
    )
  );

  return { ...totals, avgSales };
}

// Error handling with try/catch/finally
function demonstrateErrorHandling() {
  console.log("\n🚨 Error Handling Demo");
  console.log("-----------------------");

  const testInputs = [
    "Valid text with email: user@test.com and phone: 555-1234",
    null,
    "Another valid entry",
    undefined,
    42,
  ];

  let processed = 0;
  const results: string[] = [];
  const errors: string[] = [];

  for (let i = 0; i < testInputs.length; i++) {
    const input = testInputs[i];
    try {
      console.log(`Processing item ${i + 1}...`);
      const result = processText(input);
      results.push(result);
      processed++;
    } catch (error) {
      // Type narrowing in catch blocks
      let errorMsg = "Unknown error";
      if (error instanceof TypeError) {
        errorMsg = `Type error: ${error.message}`;
      } else if (typeof error === "object" && error !== null) {
        errorMsg = error.toString();
      }

      errors.push(`Item ${i + 1}: ${errorMsg}`);
    } finally {
      console.log(`Completed item ${i + 1}`);
    }
  }

  const summary = {
    processed: results.length,
    failed: errors.length,
    total: testInputs.length,
    successRate: Math.round((processed / testInputs.length) * 100),
  };

  console.log("Processing Summary:", JSON.stringify(summary, null, 2));
  console.log("Errors:", JSON.stringify(errors, null, 2));

  return { results, errors, summary };
}

// Advanced template literals and reporting
function generateReport(
  userAnalysis: any,
  salesAnalysis: any,
  errorResults: any
): string {
  console.log("\n📋 Final Report");
  console.log("================");

  const now = new Date();
  const timestamp = now.toISOString();

  // Complex template literal with multiple interpolations
  const report = `
PASERATI POWER SHOWCASE REPORT
==============================
Generated: ${timestamp}

USER ANALYTICS:
- Total Users: ${userAnalysis.results.length}
- Active TypeScript Developers: ${userAnalysis.summary.activeTypeScriptUsers}
- Senior vs Junior: ${userAnalysis.summary.senior || 0} / ${
    userAnalysis.summary.junior || 0
  }

SALES PERFORMANCE:
- Total Sales: $${salesAnalysis.totalSales.toLocaleString()}
- Average per Quarter: $${salesAnalysis.avgSales.toLocaleString()}
- Quarters Tracked: ${salesAnalysis.quarters.length}

ERROR HANDLING RESULTS:
- Success Rate: ${errorResults.summary.successRate}%
- Items Processed: ${errorResults.summary.processed}/${
    errorResults.summary.total
  }
- Errors: ${errorResults.errors.length}

${
  errorResults.errors.length > 0
    ? `Error Details:\n${errorResults.errors
        .map((err: string, i: number) => `  ${i + 1}. ${err}`)
        .join("\n")}`
    : "Perfect execution - no errors! 🎉"
}

FEATURES DEMONSTRATED:
✅ Generics with constraints and type inference
✅ Complex destructuring (objects and arrays)
✅ Spread syntax (arrays and objects)
✅ Type narrowing with typeof guards
✅ Regular expressions for text processing
✅ Try/catch/finally error handling
✅ Higher-order functions and method chaining
✅ Template literals with complex interpolation
✅ Functional programming patterns

---
Generated by Paserati TypeScript Runtime
All ${
    userAnalysis.results.length +
    salesAnalysis.quarters.length +
    errorResults.summary.total
  } data points processed successfully!
    `.trim();

  console.log(report);
  return report;
}

// Main execution with comprehensive error handling
function main() {
  console.time("Showcase Execution Time");

  try {
    // Execute all demonstrations
    const userAnalysis = analyzeUsers();
    const salesAnalysis = processSalesData();
    const errorResults = demonstrateErrorHandling();

    // Generate comprehensive report
    const report = generateReport(userAnalysis, salesAnalysis, errorResults);

    console.log(
      `\n✨ Showcase completed! Report has ${report.split("\n").length} lines.`
    );

    return {
      success: true,
      userAnalysis,
      salesAnalysis,
      errorResults,
      reportLength: report.length,
    };
  } catch (error) {
    console.error("💥 Critical error:", error);
    throw error;
  } finally {
    console.timeEnd("Showcase Execution Time");
    console.log("🎯 Paserati Power Showcase Complete!");
  }
}

// Execute the showcase
const result = main();
console.log(
  "\n🚀 Final result:",
  JSON.stringify(
    {
      success: result.success,
      dataPointsProcessed:
        result.userAnalysis.results.length +
        result.salesAnalysis.quarters.length,
      reportSize: result.reportLength,
    },
    null,
    2
  )
);
