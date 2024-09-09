// SPDX-License-Identifier: MIT
pragma solidity 0.8.15;

import { L1Block } from "src/L2/L1Block.sol";
import { Constants } from "src/libraries/Constants.sol";

contract Ethereum {
    error NotDepositor();
    error NotParallelArrays();

    L1Block internal constant L1_BLOCK = L1Block(0x4200000000000000000000000000000000000015);

    mapping(address => mapping(bytes32 => bytes)) internal _state;
    mapping(bytes32 => bytes32) internal latestBlockHashForCalldata;

    function setState(address[] calldata _targets, bytes[] calldata _retdata, bytes32[] calldata _hashedCalldata) external {
        if (msg.sender != Constants.DEPOSITOR_ACCOUNT) revert NotDepositor();

        uint256 _targetsLength = _targets.length;
        if (_targetsLength != _retdata.length || _targetsLength != _hashedCalldata.length) revert NotParallelArrays();
        for (uint256 i; i < _targetsLength; i++) {
            _state[_targets[i]][_hashedCalldata[i]] = _retdata[i];
            latestBlockHashForCalldata[_hashedCalldata[i]] = L1_BLOCK.hash();
        }
    }

    function call(address _target, bytes calldata _calldata) external view returns (bytes memory _retdata, bool _isStale) {
        bytes32 _hashedCalldata = keccak256(_calldata);
        _retdata = _state[_target][_hashedCalldata];
        _isStale = L1_BLOCK.hash() != latestBlockHashForCalldata[_hashedCalldata];
    }
}