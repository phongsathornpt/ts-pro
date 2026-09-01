function boolChannelToNumber(value: boolean): number {
  if (value) return 1;
  return 0;
}

const boolChannel = channel<boolean>(1);
console.log(boolChannelToNumber(channelTrySend(boolChannel, true)));
console.log(boolChannelToNumber(channelTryRecvOr(boolChannel, false)));
console.log(boolChannelToNumber(channelTryRecvOr(boolChannel, false)));
