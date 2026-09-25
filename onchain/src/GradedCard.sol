// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {ERC721} from "@openzeppelin/contracts/token/ERC721/ERC721.sol";
import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";

/// @title GradedCard — testnet stand-in for a tokenized, vaulted graded card
/// @notice In production, cards are tokenized by vaulting platforms. This contract lets
///         the demo mint a token that points at a real slab (grader + cert number).
///         Anyone can also claim a DEMO COPY of a listed slab to try borrowing: a demo copy
///         points at the same cert (so it uses the same price certificate) and is flagged
///         `isDemo` so it can never be mistaken for the real tokenized slab.
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

    // ----- demo copies -----
    Card[] private _demoCards;                    // slabs anyone can claim a demo copy of
    mapping(uint256 => bool) public isDemo;       // tokenId => demo copy
    mapping(address => uint256) public lastDemoClaim;
    uint256 public demoCooldown = 10 minutes;

    event DemoCardAdded(uint256 indexed index, string grader, string certId);
    event DemoClaimed(address indexed to, uint256 indexed tokenId, uint256 indexed index);

    error AlreadyMinted();
    error BadDemoIndex();
    error DemoCooldown(uint256 availableAt);

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

    // ----- demo -----

    function addDemoCard(string calldata grader, string calldata certId, string calldata grade, string calldata name)
        external onlyOwner returns (uint256 index)
    {
        index = _demoCards.length;
        _demoCards.push(Card(grader, certId, grade, name));
        emit DemoCardAdded(index, grader, certId);
    }

    function setDemoCooldown(uint256 s) external onlyOwner { demoCooldown = s; }

    function demoCardCount() external view returns (uint256) { return _demoCards.length; }

    function demoCard(uint256 index) external view returns (Card memory) {
        if (index >= _demoCards.length) revert BadDemoIndex();
        return _demoCards[index];
    }

    /// @notice Claim a demo copy of listed slab `index`. Rate-limited per wallet.
    function claimDemo(uint256 index) external returns (uint256 id) {
        if (index >= _demoCards.length) revert BadDemoIndex();
        uint256 last = lastDemoClaim[msg.sender];
        if (last != 0 && block.timestamp < last + demoCooldown) revert DemoCooldown(last + demoCooldown);
        lastDemoClaim[msg.sender] = block.timestamp;
        id = nextId++;
        _cards[id] = _demoCards[index];
        isDemo[id] = true;
        _safeMint(msg.sender, id);
        emit DemoClaimed(msg.sender, id, index);
    }

    // ----- views -----

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
