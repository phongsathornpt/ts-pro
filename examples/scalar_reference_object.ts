interface ScalarReferenceHolder {
  value: string;
}

function scalarReferenceValue(flag: boolean): string {
  const holder: ScalarReferenceHolder = { value: "start" };
  if (flag) {
    holder.value = "alpha";
  } else {
    holder.value = "beta";
  }
  return holder.value;
}

console.log(scalarReferenceValue(false));
