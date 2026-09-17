// A computed property key ([expr]: target) nested inside another object
// pattern's own sub-pattern must be accepted the same way a top-level
// computed key already is. See paserati#473.
// expect: TA-value

function pick(obj: any, key: string = "typeAnnotation") {
  let {
    node: { [key]: value },
  } = obj;
  return value;
}

pick({ node: { typeAnnotation: "TA-value" } });
