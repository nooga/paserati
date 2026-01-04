// Comprehensive test for toJSONSchema() features
// expect: all features working

// === Test $schema header ===
const primitiveSchema = Paserati.reflect<string>().toJSONSchema();
const has$schema = primitiveSchema.$schema === "https://json-schema.org/draft/2020-12/schema";

// === Test enum optimization for string literals ===
type Status = "active" | "inactive" | "pending";
const statusSchema = Paserati.reflect<Status>().toJSONSchema();
const hasStringEnum = statusSchema.type === "string" &&
    statusSchema.enum &&
    statusSchema.enum.length === 3;

// === Test enum optimization for number literals ===
type Priority = 1 | 2 | 3;
const prioritySchema = Paserati.reflect<Priority>().toJSONSchema();
const hasNumberEnum = prioritySchema.type === "number" &&
    prioritySchema.enum &&
    prioritySchema.enum.length === 3;

// === Test additionalProperties for index signatures ===
interface StringToNumberMap {
    [key: string]: number;
}
const mapSchema = Paserati.reflect<StringToNumberMap>().toJSONSchema();
const hasAdditionalProps = mapSchema.type === "object" &&
    mapSchema.additionalProperties &&
    mapSchema.additionalProperties.type === "number";

// === Test mixed properties with index signature ===
interface Config {
    name: string;
    [key: string]: string | number;
}
const configSchema = Paserati.reflect<Config>().toJSONSchema();
const hasMixedProps = configSchema.type === "object" &&
    configSchema.properties &&
    configSchema.properties.name &&
    configSchema.additionalProperties;

// === Test nested types ===
interface Address {
    street: string;
    city: string;
}
interface Person {
    name: string;
    address: Address;
}
const personSchema = Paserati.reflect<Person>().toJSONSchema();
const hasNestedObject = personSchema.properties &&
    personSchema.properties.address &&
    personSchema.properties.address.type === "object";

// === Verify all features ===
const allWorking = has$schema &&
    hasStringEnum &&
    hasNumberEnum &&
    hasAdditionalProps &&
    hasMixedProps &&
    hasNestedObject;

allWorking ? "all features working" : "some features failed";
