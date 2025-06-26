// Data Processing Showcase - Demonstrating Paserati's Advanced Features
// This example combines generics, destructuring, spread syntax, functional programming,
// type narrowing, regular expressions, and error handling in a realistic scenario

console.log("🚀 Paserati Data Processing Showcase");
console.log("=====================================");

// Generic data structures with constraints
interface DataPoint<T extends string | number> {
  id: string;
  value: T;
  timestamp: number;
  metadata?: { [key: string]: unknown };
}

interface ProcessingResult<T> {
  processed: T[];
  errors: string[];
  summary: {
    total: number;
    successful: number;
    failed: number;
  };
}

// Advanced generic utility types
type Transformer<TInput, TOutput> = (input: TInput) => TOutput;
type Validator<T> = (value: T) => boolean;
type Aggregator<T, TResult> = (acc: TResult, current: T) => TResult;

// Sample datasets with complex destructuring
const rawUserData = [
  {
    user: { name: "Alice", age: 28, skills: ["TypeScript", "Go", "Python"] },
    active: true,
  },
  {
    user: { name: "Bob", age: 32, skills: ["JavaScript", "Rust"] },
    active: false,
  },
  {
    user: {
      name: "Charlie",
      age: 24,
      skills: ["TypeScript", "C++", "WebAssembly"],
    },
    active: true,
  },
];

const salesData: DataPoint<number>[] = [
  { id: "Q1-2024", value: 125000, timestamp: Date.now() - 86400000 },
  { id: "Q2-2024", value: 98000, timestamp: Date.now() - 43200000 },
  { id: "Q3-2024", value: 152000, timestamp: Date.now() - 21600000 },
];

// Generic data processor with advanced features
class DataProcessor<T> {
  private data: T[];
  private transformers: Transformer<T, T>[] = [];

  constructor(initialData: T[] = []) {
    this.data = [...initialData]; // Spread syntax for immutability
  }

  // Method chaining with generics
  transform<U>(transformer: Transformer<T, U>): DataProcessor<U> {
    const transformed = this.data.map(transformer);
    return new DataProcessor(transformed);
  }

  filter(predicate: Validator<T>): DataProcessor<T> {
    return new DataProcessor(this.data.filter(predicate));
  }

  // Advanced aggregation with type inference
  aggregate<TResult>(
    initialValue: TResult,
    aggregator: Aggregator<T, TResult>
  ): TResult {
    return this.data.reduce(aggregator, initialValue);
  }

  // Destructuring in method parameters
  processInBatches({
    batchSize = 10,
    maxRetries = 3,
  }: { batchSize?: number; maxRetries?: number } = {}): ProcessingResult<T> {
    const errors: string[] = [];
    const processed: T[] = [];

    for (let i = 0; i < this.data.length; i += batchSize) {
      const batch = this.data.slice(i, i + batchSize);

      try {
        // Spread syntax in array operations
        processed.push(...batch);
      } catch (error) {
        if (typeof error === "object" && error !== null) {
          errors.push(`Batch ${i / batchSize + 1} failed: ${error.toString()}`);
        } else {
          errors.push(`Batch ${i / batchSize + 1} failed: Unknown error`);
        }
      }
    }

    return {
      processed,
      errors,
      summary: {
        total: this.data.length,
        successful: processed.length,
        failed: this.data.length - processed.length,
      },
    };
  }

  getData(): T[] {
    return [...this.data]; // Defensive copying with spread
  }
}

// Text processing with regular expressions and type narrowing
function processTextData(input: unknown): string {
  // Type narrowing with typeof guards
  if (typeof input !== "string") {
    throw new TypeError(`Expected string, got ${typeof input}`);
  }

  // Regular expression processing
  const emailRegex = /[\w.-]+@[\w.-]+\.\w+/g;
  const phoneRegex = /\(?[\d\s-()]{10,}\)?/g;

  let processed = input;

  // Extract and mask sensitive data
  const emails = processed.match(emailRegex) || [];
  const phones = processed.match(phoneRegex) || [];

  // Replace with masked versions
  processed = processed.replace(emailRegex, "[EMAIL]");
  processed = processed.replace(phoneRegex, "[PHONE]");

  return processed;
}

// Complex destructuring patterns
function analyzeUserData() {
  console.log("\n📊 User Data Analysis");
  console.log("---------------------");

  // Destructuring with rest patterns and defaults
  const results = rawUserData.map(
    ({ user: { name, age, skills = [] }, active = false }) => {
      // Further destructuring of arrays
      const [primarySkill, ...otherSkills] = skills;

      return {
        name,
        demographic: age >= 30 ? "senior" : "junior",
        expertise: {
          primary: primarySkill || "none",
          secondary: otherSkills.length,
          hasTypeScript: skills.includes("TypeScript"),
        },
        status: active ? "active" : "inactive",
      };
    }
  );

  // Spread syntax in object literals with computed properties
  const summary = results.reduce(
    (acc, user) => ({
      ...acc,
      [user.demographic]: (acc[user.demographic] || 0) + 1,
      totalActiveTypeScriptUsers:
        acc.totalActiveTypeScriptUsers +
        (user.status === "active" && user.expertise.hasTypeScript ? 1 : 0),
    }),
    { totalActiveTypeScriptUsers: 0 } as { [key: string]: number }
  );

  console.log("User Analysis Results:", JSON.stringify(results, null, 2));
  console.log("Summary:", JSON.stringify(summary, null, 2));

  return { results, summary };
}

// Higher-order function composition with generics
function compose<T, U, V>(f: (x: U) => V, g: (x: T) => U): (x: T) => V {
  return (x: T) => f(g(x));
}

// Practical pipeline example
function createSalesAnalysisPipeline() {
  console.log("\n💰 Sales Data Processing Pipeline");
  console.log("----------------------------------");

  const processor = new DataProcessor(salesData);

  // Transform data with method chaining
  const enriched = processor
    .transform(({ id, value, timestamp, ...rest }) => ({
      id,
      value,
      timestamp,
      quarter: id.split("-")[0],
      year: parseInt(id.split("-")[1]),
      valueInK: Math.round(value / 1000),
      ...rest, // Spread remaining properties
    }))
    .filter((data) => data.year >= 2024);

  // Aggregate with advanced patterns
  const totals = enriched.aggregate(
    { totalSales: 0, avgSales: 0, quarters: [] as string[] },
    (acc, { value, quarter }) => ({
      totalSales: acc.totalSales + value,
      avgSales: 0, // Will calculate after
      quarters: acc.quarters.includes(quarter)
        ? acc.quarters
        : [...acc.quarters, quarter],
    })
  );

  totals.avgSales = Math.round(totals.totalSales / totals.quarters.length);

  console.log("Sales Analysis:", JSON.stringify(totals, null, 2));

  return totals;
}

// Error handling showcase with finally blocks
function demonstrateErrorHandling(): ProcessingResult<string> {
  console.log("\n🚨 Error Handling Demonstration");
  console.log("--------------------------------");

  const testData = [
    "Valid data with email: user@example.com and phone: 555-0123",
    null, // This will cause an error
    "Another valid entry",
    undefined, // This will also cause an error
    "Final entry with phone: (555) 987-6543",
  ];

  let processedCount = 0;
  const result: ProcessingResult<string> = {
    processed: [],
    errors: [],
    summary: { total: testData.length, successful: 0, failed: 0 },
  };

  for (let i = 0; i < testData.length; i++) {
    const item = testData[i];
    try {
      console.log(`Processing item ${i + 1}...`);
      const processed = processTextData(item);
      result.processed.push(processed);
      processedCount++;
    } catch (error) {
      // Type narrowing in catch blocks
      let errorMessage = "Unknown error";
      if (error instanceof TypeError) {
        errorMessage = `Type error: ${error.message}`;
      } else if (
        typeof error === "object" &&
        error !== null &&
        "message" in error
      ) {
        errorMessage = String(error.message);
      }

      result.errors.push(`Item ${i + 1}: ${errorMessage}`);
    } finally {
      // Finally block always executes
      console.log(`Completed processing item ${i + 1}`);
    }
  }

  result.summary.successful = processedCount;
  result.summary.failed = testData.length - processedCount;

  console.log("Processing Results:", JSON.stringify(result, null, 2));
  return result;
}

// Template literal showcase with advanced interpolation
function generateReport(
  userData: any,
  salesData: any,
  errorResults: ProcessingResult<string>
) {
  console.log("\n📋 Generated Report");
  console.log("===================");

  const timestamp = new Date().toISOString();

  // Complex template literal with multiple interpolations
  const report = `
PASERATI DATA PROCESSING REPORT
Generated: ${timestamp}

USER ANALYTICS:
- Total Users Analyzed: ${userData.results.length}
- Active TypeScript Users: ${userData.summary.totalActiveTypeScriptUsers}
- Senior Developers: ${userData.summary.senior || 0}
- Junior Developers: ${userData.summary.junior || 0}

SALES PERFORMANCE:
- Total Sales: $${salesData.totalSales.toLocaleString()}
- Average per Quarter: $${salesData.avgSales.toLocaleString()}
- Quarters Analyzed: ${salesData.quarters.size}

ERROR HANDLING RESULTS:
- Items Processed: ${errorResults.summary.successful}/${
    errorResults.summary.total
  }
- Success Rate: ${Math.round(
    (errorResults.summary.successful / errorResults.summary.total) * 100
  )}%
- Errors Encountered: ${errorResults.errors.length}

${
  errorResults.errors.length > 0
    ? `ERROR DETAILS:\n${errorResults.errors
        .map((err, i) => `  ${i + 1}. ${err}`)
        .join("\n")}`
    : "No errors encountered! 🎉"
}

---
Report generated by Paserati TypeScript Runtime
    `.trim();

  console.log(report);
  return report;
}

// Main execution with comprehensive feature demonstration
function main() {
  try {
    console.time("Total Processing Time");

    // Execute all demonstrations
    const userAnalysis = analyzeUserData();
    const salesAnalysis = createSalesAnalysisPipeline();
    const errorResults = demonstrateErrorHandling();

    // Generate comprehensive report
    const finalReport = generateReport(
      userAnalysis,
      salesAnalysis,
      errorResults
    );

    console.log("\n✅ All demonstrations completed successfully!");
    console.log(
      `📊 Report contains ${finalReport.split("\n").length} lines of analysis`
    );
  } catch (error) {
    console.error("❌ Critical error in main execution:", error);
    throw error;
  } finally {
    console.timeEnd("Total Processing Time");
    console.log(
      "\n🏁 Showcase completed - demonstrating the power of Paserati!"
    );
  }
}

// Execute the showcase
main();
