interface DynamicChild {
  value: number;
}

interface DynamicHolder {
  count: number;
  label: string;
  active: boolean;
  child: DynamicChild;
}

const typed: DynamicHolder = {
  count: 42,
  label: "shape-aware",
  active: true,
  child: { value: 7 },
};
const dynamic: any = typed;

console.log(dynamic.count);
console.log(dynamic.label);
console.log(dynamic.active);
console.log(dynamic.child.value);
console.log(dynamic.missing);
