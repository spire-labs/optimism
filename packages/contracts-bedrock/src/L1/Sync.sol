// SPDX-License-Identifier: MIT
pragma solidity 0.8.15;


contract Sync {

    error NotParallelArrays();

    // NOTE: This is very expensive, for poc is fine, in production investigate optimizations
    bytes[] internal _calldata;
    bytes[] internal _retdata;
    address[] internal _targets;

    // NOTE: For the sake of poc this is left open for anyone to call, in production this should be changed
    // NOTE: __retdata is unneccesary to save as we get it from sync() need to change this later
    function setSync(bytes[] memory __calldata, bytes[]  memory __retdata, address[] memory __targets) external {
         uint256 __calldataLength = __calldata.length;
        if (__calldataLength != __retdata.length || __calldataLength != __targets.length) revert NotParallelArrays();

        _calldata = __calldata;
        _retdata = __retdata;
        _targets = __targets;
    }

    // NOTE: This function is only supposed to be called within the context of an `eth_call`
    function sync() external returns (address[] memory __targets, bytes[] memory __retdata, bytes32[] memory __hashedCalldata) {
        uint256 _calldataLength = _calldata.length;
        if (_calldataLength == 0) return (__targets, __retdata, __hashedCalldata);

        __targets = new address[](_calldataLength);
        __retdata = new bytes[](_calldataLength);
        __hashedCalldata = new bytes32[](_calldataLength);

        // WARNING: This is broken and will potentially break for calldata's and return data's that are larger than 32 bytes
        // Needs to be fixed for prod, fine for PoC
        for (uint256 i; i < _calldataLength; i++) {
            __hashedCalldata[i] = keccak256(_calldata[i]);
            // NOTE: We dont care if it reverts or not, if it does, sync should show that it reverted with the retdata
            (,__retdata[i]) = _targets[i].call(_calldata[i]);
            __targets[i] = _targets[i];
        }
    }
}