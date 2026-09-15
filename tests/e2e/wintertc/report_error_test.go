package wintertc_test

import "testing"

func TestLinuxAMD64WinterTCReportErrorCallsGlobalOnError(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_report_error_onerror",
		source: `
let observed = 0;
const listener = (event: Event): void => {
  observed = observed + 1;
  console.log(event.type);
};

globalThis.addEventListener("error", listener);
globalThis.onerror = (
  message: string,
  source: string,
  line: number,
  column: number,
  error: any,
): any => {
  console.log(message);
  console.log(source === "");
  console.log(line);
  console.log(column);
  console.log(error);
  return true;
};

reportError("boom");
globalThis.removeEventListener("error", listener);
globalThis.onerror = null;
reportError("ignored");
console.log(observed);
console.log("done");
`,
		expected: "error\nboom\ntrue\n0\n0\nboom\n1\ndone\n",
	})
}
