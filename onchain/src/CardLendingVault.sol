// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {IERC721} from "@openzeppelin/contracts/token/ERC721/IERC721.sol";
import {IERC721Receiver} from "@openzeppelin/contracts/token/ERC721/IERC721Receiver.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import {AppraisalRegistry} from "./AppraisalRegistry.sol";
import {GradedCard} from "./GradedCard.sol";

/// @title CardLendingVault — borrow stablecoins against a graded card
/// @notice Deposit a GradedCard, the vault reads its Gacha Galaxy price certificate from
///         AppraisalRegistry, and you can borrow up to fmvLow x ltvBps. If a fresh appraisal
///         drops the card's value so the loan exceeds the liquidation threshold, anyone can
///         repay the debt and take the card.
/// @dev Demo scope: no interest accrual; single loan asset. Values are USD cents in the
///      registry; the loan asset has 6 decimals, so 1 cent = 1e4 units.
contract CardLendingVault is IERC721Receiver, Ownable, ReentrancyGuard {
    using SafeERC20 for IERC20;

    AppraisalRegistry public immutable registry;
    GradedCard public immutable cards;
    IERC20 public immutable loanAsset;

    uint256 public constant CENT = 1e4;             // 6-decimal asset units per USD cent
    uint256 public maxAppraisalAge = 7 days;         // certificate must be this fresh to borrow
    uint16 public liquidationBufferBps = 1000;       // liquidate at ltv + 10 points

    struct Position { address owner; uint256 debt; }
    mapping(uint256 => Position) public positions;   // tokenId => position

    event Deposited(uint256 indexed tokenId, address indexed owner);
    event Borrowed(uint256 indexed tokenId, address indexed owner, uint256 amount);
    event Repaid(uint256 indexed tokenId, address indexed payer, uint256 amount);
    event Withdrawn(uint256 indexed tokenId, address indexed owner);
    event Liquidated(uint256 indexed tokenId, address indexed liquidator, uint256 debt);
    event ParamsUpdated(uint256 maxAppraisalAge, uint16 liquidationBufferBps);

    error NotOwner();
    error NotDeposited();
    error StaleAppraisal();
    error NotEligible();
    error ExceedsLimit(uint256 requested, uint256 available);
    error OutstandingDebt();
    error Healthy();
    error WrongCollection();

    constructor(address owner_, AppraisalRegistry registry_, GradedCard cards_, IERC20 loanAsset_) Ownable(owner_) {
        registry = registry_;
        cards = cards_;
        loanAsset = loanAsset_;
    }

    // ----- views -----

    /// @notice Max total debt allowed for a card right now (0 if stale / ineligible).
    function borrowLimit(uint256 tokenId) public view returns (uint256) {
        AppraisalRegistry.Appraisal memory a = registry.getAppraisal(cards.certKeyOf(tokenId));
        if (!a.eligible || a.risk == AppraisalRegistry.Risk.REJECT) return 0;
        if (block.timestamp > uint256(a.appraisedAt) + maxAppraisalAge) return 0;
        return uint256(a.fmvLow) * CENT * a.ltvBps / 10_000;
    }

    /// @notice Debt level at which the position can be liquidated.
    function liquidationThreshold(uint256 tokenId) public view returns (uint256) {
        AppraisalRegistry.Appraisal memory a = registry.getAppraisal(cards.certKeyOf(tokenId));
        uint256 bps = uint256(a.ltvBps) + liquidationBufferBps;
        if (!a.eligible || a.risk == AppraisalRegistry.Risk.REJECT) bps = 0;
        if (bps > 10_000) bps = 10_000;
        return uint256(a.fmvLow) * CENT * bps / 10_000;
    }

    function isLiquidatable(uint256 tokenId) public view returns (bool) {
        Position memory p = positions[tokenId];
        return p.owner != address(0) && p.debt > 0 && p.debt > liquidationThreshold(tokenId);
    }

    // ----- actions -----

    function deposit(uint256 tokenId) external nonReentrant {
        cards.safeTransferFrom(msg.sender, address(this), tokenId);
    }

    function onERC721Received(address, address from, uint256 tokenId, bytes calldata) external returns (bytes4) {
        if (msg.sender != address(cards)) revert WrongCollection();
        positions[tokenId] = Position({owner: from, debt: 0});
        emit Deposited(tokenId, from);
        return IERC721Receiver.onERC721Received.selector;
    }

    function borrow(uint256 tokenId, uint256 amount) external nonReentrant {
        Position storage p = positions[tokenId];
        if (p.owner == address(0)) revert NotDeposited();
        if (p.owner != msg.sender) revert NotOwner();

        AppraisalRegistry.Appraisal memory a = registry.getAppraisal(cards.certKeyOf(tokenId));
        if (!a.eligible || a.risk == AppraisalRegistry.Risk.REJECT) revert NotEligible();
        if (block.timestamp > uint256(a.appraisedAt) + maxAppraisalAge) revert StaleAppraisal();

        uint256 limit = uint256(a.fmvLow) * CENT * a.ltvBps / 10_000;
        uint256 available = limit > p.debt ? limit - p.debt : 0;
        if (amount > available) revert ExceedsLimit(amount, available);

        p.debt += amount;
        loanAsset.safeTransfer(msg.sender, amount);
        emit Borrowed(tokenId, msg.sender, amount);
    }

    function repay(uint256 tokenId, uint256 amount) external nonReentrant {
        Position storage p = positions[tokenId];
        if (p.owner == address(0)) revert NotDeposited();
        if (amount > p.debt) amount = p.debt;
        p.debt -= amount;
        loanAsset.safeTransferFrom(msg.sender, address(this), amount);
        emit Repaid(tokenId, msg.sender, amount);
    }

    function withdraw(uint256 tokenId) external nonReentrant {
        Position memory p = positions[tokenId];
        if (p.owner == address(0)) revert NotDeposited();
        if (p.owner != msg.sender) revert NotOwner();
        if (p.debt != 0) revert OutstandingDebt();
        delete positions[tokenId];
        cards.safeTransferFrom(address(this), msg.sender, tokenId);
        emit Withdrawn(tokenId, msg.sender);
    }

    /// @notice If a new appraisal pushes debt above the threshold, anyone can repay and take the card.
    function liquidate(uint256 tokenId) external nonReentrant {
        if (!isLiquidatable(tokenId)) revert Healthy();
        uint256 debt = positions[tokenId].debt;
        delete positions[tokenId];
        loanAsset.safeTransferFrom(msg.sender, address(this), debt);
        cards.safeTransferFrom(address(this), msg.sender, tokenId);
        emit Liquidated(tokenId, msg.sender, debt);
    }

    // ----- admin -----

    function setParams(uint256 maxAge, uint16 bufferBps) external onlyOwner {
        maxAppraisalAge = maxAge;
        liquidationBufferBps = bufferBps;
        emit ParamsUpdated(maxAge, bufferBps);
    }
}
