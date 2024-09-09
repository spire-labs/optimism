// SPDX-License-Identifier: MIT
pragma solidity 0.8.15;


contract Sync {
    error NotParallelArrays();

    // NOTE: This is very expensive, for poc is fine, in production investigate optimizations
    bytes[] internal _calldata;
    bytes[] internal _retdata;
    address[] internal _targets;

    // NOTE: For the sake of poc this is left open for anyone to call, in production this should be changed
    function setSync(bytes[] calldata __calldata, bytes[] calldata __retdata, address[] calldata __targets) external {
         uint256 __calldataLength = __calldata.length;
        if (__calldataLength != __retdata.length || __calldataLength != __targets.length) revert NotParallelArrays();

        for (uint256 i; i < __calldataLength; i++) {
            _calldata[i] = __calldata[i];
            _retdata[i] = __retdata[i];
            _targets[i] = __targets[i];
        }
    }

    // NOTE: This function is only supposed to be called within the context of an `eth_call`
    function sync() external returns (address[] memory __targets, bytes[] memory __retdata, bytes32[] memory __hashedCalldata) {
        uint256 _calldataLength = _calldata.length;
        if (_calldataLength == 0) return (__targets, __retdata, __hashedCalldata);

        for (uint256 i; i < _calldataLength; i++) {
            __hashedCalldata[i] = keccak256(_calldata[i]);
            // NOTE: We dont care if it reverts or not, if it does, sync should show that it reverted with the retdata
            (,_retdata[i]) = _targets[i].call(_calldata[i]);
        }
    }
}