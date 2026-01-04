// Test Paserati.reflect<T>().toJSONSchema()
// expect: {"$schema":"https://json-schema.org/draft/2020-12/schema","type":"string"}

const stringType = Paserati.reflect<string>();
const schema = stringType.toJSONSchema();
JSON.stringify(schema);
