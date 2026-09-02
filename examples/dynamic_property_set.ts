interface DynamicSetChild {
  value: number;
}

interface DynamicSetHolder {
  count: number;
  label: string;
  active: boolean;
  child: DynamicSetChild;
}

function runDynamicPropertySet(): void {
  const setTyped: DynamicSetHolder = {
    count: 1,
    label: "before",
    active: true,
    child: { value: 2 },
  };
  const replacement: DynamicSetChild = { value: 77 };
  const setDynamic: any = setTyped;

  let setChurn = "";
  for (let i: number = 0; i < 512; i++) {
    setChurn = setChurn + "0123456789abcdef";
  }

  setDynamic.count = 99;
  setDynamic.label = "after";
  setDynamic.active = false;
  setDynamic.child = replacement;

  for (let i: number = 0; i < 512; i++) {
    setChurn = setChurn + "fedcba9876543210";
  }

  console.log(setTyped.count);
  console.log(setTyped.label);
  console.log(setDynamic.active);
  console.log(setTyped.child.value);
}

runDynamicPropertySet();
