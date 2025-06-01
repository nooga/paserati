// expect: undefined

interface SimpleInstance {
  x: string;
}

interface SimpleConstructor {
  new (): SimpleInstance;
}

// Just return the interface type to show it parsed correctly
let constructorType: SimpleConstructor;
constructorType;
