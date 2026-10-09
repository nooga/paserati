// expect_compile_error: Type 'number' is not assignable to type 'OrderDefinition'
interface OrderDefinition { item: string; quantity?: number }
const x: OrderDefinition = 5;
