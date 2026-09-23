// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {ERC721} from "@openzeppelin/contracts/token/ERC721/ERC721.sol";
import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";

/// @title GradedCard — testnet stand-in for a tokenized, vaulted graded card
/// @notice In production, cards are tokenized by vaulting platforms. This contract lets
///         the demo mint a token that points at a real slab (grader + cert number).
contract GradedCard is ERC721, Ownable {
    struct Card {
        string grader;   // e.g. "PSA"
        string certId;   // number printed on the slab
        string grade;    // e.g. "10"
        string name;     // e.g. "Charizard - Base Set Holo"
    }

    uint256 public nextId = 1;
    mapping(uint256 => Card) private _cards;
    mapping(bytes32 => bool) public certMinted;

    error AlreadyMinted();

    constructor(address owner_) ERC721("Gacha Galaxy Graded Card (Testnet)", "gCARD") Ownable(owner_) {}

    function mint(address to, string calldata grader, string calldata certId, string calldata grade, string calldata name)
        external onlyOwner returns (uint256 id)
    {
        bytes32 k = keccak256(abi.encode(grader, certId));
        if (certMinted[k]) revert AlreadyMinted();
        certMinted[k] = true;
        id = nextId++;
        _cards[id] = Card(grader, certId, grade, name);
        _safeMint(to, id);
    }

    function card(uint256 id) external view returns (Card memory) {
        _requireOwned(id);
        return _cards[id];
    }

    function certKeyOf(uint256 id) external view returns (bytes32) {
        _requireOwned(id);
        Card storage c = _cards[id];
        return keccak256(abi.encode(c.grader, c.certId));
    }
}
