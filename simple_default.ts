function outer(multiplier = null): number {
  function inner(): number {
    return multiplier || 5;
  }
  return inner();
}
outer();
