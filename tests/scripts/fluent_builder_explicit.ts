// Fluent builder with explicit return type annotations
// expect: Query: SELECT * FROM users WHERE active = true

class QueryBuilder {
  private query: string = "";

  select(fields: string): QueryBuilder {
    this.query = "SELECT " + fields;
    return this;
  }

  from(table: string): QueryBuilder {
    this.query = this.query + " FROM " + table;
    return this;
  }

  where(condition: string): QueryBuilder {
    this.query = this.query + " WHERE " + condition;
    return this;
  }

  build(): string {
    return "Query: " + this.query;
  }
}

const result = new QueryBuilder()
  .select("*")
  .from("users")
  .where("active = true")
  .build();

result;
