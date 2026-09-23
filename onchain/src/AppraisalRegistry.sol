// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {AccessControl} from "@openzeppelin/contracts/access/AccessControl.sol";

/// @title AppraisalRegistry — Gacha Galaxy price certificates on Robinhood Chain
/// @notice Stores the latest appraisal ("price certificate") for a graded collectible,
///         keyed by grader + cert number (the number printed on the slab).
///         Lending protocols read these certificates to decide how much can be borrowed.
contract AppraisalRegistry is AccessControl {
    bytes32 public constant ATTESTER_ROLE = keccak256("ATTESTER_ROLE");

    enum Confidence { NONE, NARRATIVE, MODEST, REAL }
    enum Risk { NONE, REJECT, C, B, A }

    struct Appraisal {
        uint64 fmvLow;        // USD cents
        uint64 fmvPoint;      // USD cents
        uint64 fmvHigh;       // USD cents
        uint16 ltvBps;        // max loan-to-value, basis points (5000 = 50%)
        uint16 confidenceScore; // 0-1000
        uint16 compCount;     // number of comparable sales used
        Confidence confidence;
        Risk risk;
        bool eligible;        // collateral-eligible
        uint40 appraisedAt;   // unix seconds
        address attester;
    }

    mapping(bytes32 => Appraisal) private _appraisals;
    mapping(bytes32 => uint256) public appraisalCount;

    event AppraisalPublished(
        bytes32 indexed certKey, string grader, string certId,
        uint64 fmvLow, uint64 fmvPoint, uint64 fmvHigh,
        uint16 ltvBps, Confidence confidence, Risk risk, bool eligible, address indexed attester
    );

    error InvalidBand();
    error InvalidLtv();
    error NotFound();

    constructor(address admin, address attester) {
        _grantRole(DEFAULT_ADMIN_ROLE, admin);
        _grantRole(ATTESTER_ROLE, attester);
    }

    function certKey(string memory grader, string memory certId) public pure returns (bytes32) {
        return keccak256(abi.encode(grader, certId));
    }

    function publish(
        string calldata grader,
        string calldata certId,
        uint64 fmvLow,
        uint64 fmvPoint,
        uint64 fmvHigh,
        uint16 ltvBps,
        uint16 confidenceScore,
        uint16 compCount,
        Confidence confidence,
        Risk risk,
        bool eligible
    ) external onlyRole(ATTESTER_ROLE) returns (bytes32 key) {
        if (fmvLow > fmvPoint || fmvPoint > fmvHigh || fmvLow == 0) revert InvalidBand();
        if (ltvBps > 9000) revert InvalidLtv();
        if (!eligible || risk == Risk.REJECT) ltvBps = 0;

        key = certKey(grader, certId);
        _appraisals[key] = Appraisal({
            fmvLow: fmvLow,
            fmvPoint: fmvPoint,
            fmvHigh: fmvHigh,
            ltvBps: ltvBps,
            confidenceScore: confidenceScore,
            compCount: compCount,
            confidence: confidence,
            risk: risk,
            eligible: eligible,
            appraisedAt: uint40(block.timestamp),
            attester: msg.sender
        });
        appraisalCount[key] += 1;
        emit AppraisalPublished(key, grader, certId, fmvLow, fmvPoint, fmvHigh, ltvBps, confidence, risk, eligible, msg.sender);
    }

    function getAppraisal(bytes32 key) public view returns (Appraisal memory a) {
        a = _appraisals[key];
        if (a.appraisedAt == 0) revert NotFound();
    }

    function getAppraisalByCert(string calldata grader, string calldata certId) external view returns (Appraisal memory) {
        return getAppraisal(certKey(grader, certId));
    }

    function exists(bytes32 key) external view returns (bool) {
        return _appraisals[key].appraisedAt != 0;
    }
}
