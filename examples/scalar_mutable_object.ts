interface ScalarCounter {
  value: number;
}

const scalarMutableCounter: ScalarCounter = { value: 0 };
scalarMutableCounter.value = 7;
console.log(scalarMutableCounter.value);
