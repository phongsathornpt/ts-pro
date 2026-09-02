interface ScalarNestedCounter {
  value: number;
}

function nestedScalarValue(outer: boolean, inner: boolean): number {
  const counter: ScalarNestedCounter = { value: 0 };
  if (outer) {
    if (inner) {
      counter.value = 7;
    } else {
      counter.value = 8;
    }
  } else {
    counter.value = 9;
  }
  return counter.value;
}

console.log(nestedScalarValue(true, false));
