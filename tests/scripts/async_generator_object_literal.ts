// Test async generator in object literal  
// expect: 1

const obj = {
  async *getValues() {
    yield 1;
    yield 2;
  }
};

const gen = obj.getValues();
gen.next().then(r => console.log(r.value));
1;
