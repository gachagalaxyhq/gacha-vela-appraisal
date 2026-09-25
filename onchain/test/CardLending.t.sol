// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test} from "forge-std/Test.sol";
import {AppraisalRegistry} from "../src/AppraisalRegistry.sol";
import {GradedCard} from "../src/GradedCard.sol";
import {TestUSD} from "../src/TestUSD.sol";
import {CardLendingVault} from "../src/CardLendingVault.sol";

contract CardLendingTest is Test {
    AppraisalRegistry reg;
    GradedCard cards;
    TestUSD usd;
    CardLendingVault vault;

    address admin = address(0xA11CE);
    address attester = address(0xA77E5);
    address alice = address(0xA1);
    address liquidator = address(0xB0B);

    uint256 tokenId;

    function setUp() public {
        vm.warp(1_790_000_000);
        reg = new AppraisalRegistry(admin, attester);
        cards = new GradedCard(admin);
        usd = new TestUSD(admin);
        vault = new CardLendingVault(admin, reg, cards, usd);

        vm.startPrank(admin);
        usd.mint(address(vault), 1_000_000e6);
        usd.mint(liquidator, 100_000e6);
        usd.mint(alice, 10_000e6);
        tokenId = cards.mint(alice, "CGC", "1401024676010", "10", "Charizard Skyridge 146/144");
        vm.stopPrank();
    }

    // $1,000 low / $1,100 point / $1,200 high, 50% LTV, tier A
    function _publish(uint64 low, uint64 point, uint64 high, uint16 ltv, bool eligible) internal {
        vm.prank(attester);
        reg.publish("CGC", "1401024676010", low, point, high, ltv, 800, 12,
            AppraisalRegistry.Confidence.REAL, eligible ? AppraisalRegistry.Risk.A : AppraisalRegistry.Risk.REJECT, eligible);
    }

    function _deposit() internal {
        vm.startPrank(alice);
        cards.approve(address(vault), tokenId);
        vault.deposit(tokenId);
        vm.stopPrank();
    }

    function test_publishAndRead() public {
        _publish(100_000, 110_000, 120_000, 5000, true);
        AppraisalRegistry.Appraisal memory a = reg.getAppraisalByCert("CGC", "1401024676010");
        assertEq(a.fmvPoint, 110_000);
        assertEq(a.ltvBps, 5000);
        assertEq(a.attester, attester);
        assertEq(reg.appraisalCount(reg.certKey("CGC", "1401024676010")), 1);
    }

    function test_onlyAttesterCanPublish() public {
        vm.expectRevert();
        reg.publish("CGC", "1", 1, 1, 1, 0, 0, 0, AppraisalRegistry.Confidence.REAL, AppraisalRegistry.Risk.A, true);
    }

    function test_rejectsBadBand() public {
        vm.prank(attester);
        vm.expectRevert(AppraisalRegistry.InvalidBand.selector);
        reg.publish("CGC", "1", 200, 100, 300, 5000, 0, 0, AppraisalRegistry.Confidence.REAL, AppraisalRegistry.Risk.A, true);
    }

    function test_ineligibleForcesZeroLtv() public {
        _publish(100_000, 110_000, 120_000, 5000, false);
        assertEq(reg.getAppraisalByCert("CGC", "1401024676010").ltvBps, 0);
    }

    function test_borrowUpToLimit() public {
        _publish(100_000, 110_000, 120_000, 5000, true); // low $1,000 x 50% = $500
        _deposit();
        assertEq(vault.borrowLimit(tokenId), 500e6);
        vm.prank(alice);
        vault.borrow(tokenId, 500e6);
        assertEq(usd.balanceOf(alice), 10_500e6);
        (, uint256 debt) = vault.positions(tokenId);
        assertEq(debt, 500e6);
    }

    function test_cannotBorrowAboveLimit() public {
        _publish(100_000, 110_000, 120_000, 5000, true);
        _deposit();
        vm.prank(alice);
        vm.expectRevert(abi.encodeWithSelector(CardLendingVault.ExceedsLimit.selector, 501e6, 500e6));
        vault.borrow(tokenId, 501e6);
    }

    function test_staleAppraisalBlocksBorrow() public {
        _publish(100_000, 110_000, 120_000, 5000, true);
        _deposit();
        vm.warp(block.timestamp + 8 days);
        vm.prank(alice);
        vm.expectRevert(CardLendingVault.StaleAppraisal.selector);
        vault.borrow(tokenId, 1e6);
    }

    function test_ineligibleBlocksBorrow() public {
        _publish(100_000, 110_000, 120_000, 5000, false);
        _deposit();
        vm.prank(alice);
        vm.expectRevert(CardLendingVault.NotEligible.selector);
        vault.borrow(tokenId, 1e6);
    }

    function test_onlyOwnerBorrows() public {
        _publish(100_000, 110_000, 120_000, 5000, true);
        _deposit();
        vm.prank(liquidator);
        vm.expectRevert(CardLendingVault.NotOwner.selector);
        vault.borrow(tokenId, 1e6);
    }

    function test_repayAndWithdraw() public {
        _publish(100_000, 110_000, 120_000, 5000, true);
        _deposit();
        vm.startPrank(alice);
        vault.borrow(tokenId, 300e6);
        vm.expectRevert(CardLendingVault.OutstandingDebt.selector);
        vault.withdraw(tokenId);
        usd.approve(address(vault), 300e6);
        vault.repay(tokenId, 300e6);
        vault.withdraw(tokenId);
        vm.stopPrank();
        assertEq(cards.ownerOf(tokenId), alice);
    }

    function test_priceDropEnablesLiquidation() public {
        _publish(100_000, 110_000, 120_000, 5000, true);
        _deposit();
        vm.prank(alice);
        vault.borrow(tokenId, 500e6);
        assertFalse(vault.isLiquidatable(tokenId)); // threshold = $1,000 x 60% = $600

        vm.prank(liquidator);
        vm.expectRevert(CardLendingVault.Healthy.selector);
        vault.liquidate(tokenId);

        // market drops: new low $700 -> threshold $420 < $500 debt
        _publish(70_000, 80_000, 90_000, 5000, true);
        assertTrue(vault.isLiquidatable(tokenId));

        vm.startPrank(liquidator);
        usd.approve(address(vault), 500e6);
        vault.liquidate(tokenId);
        vm.stopPrank();
        assertEq(cards.ownerOf(tokenId), liquidator);
    }

    function test_cannotMintSameCertTwice() public {
        vm.prank(admin);
        vm.expectRevert(GradedCard.AlreadyMinted.selector);
        cards.mint(alice, "CGC", "1401024676010", "10", "dup");
    }

    function testFuzz_borrowNeverExceedsLimit(uint64 low, uint16 ltv, uint256 amt) public {
        low = uint64(bound(low, 100, 100_000_000)); // up to $1M
        ltv = uint16(bound(ltv, 0, 9000));
        _publish(low, low, low, ltv, true);
        _deposit();
        uint256 limit = vault.borrowLimit(tokenId);
        amt = bound(amt, 0, limit + 1e6);
        vm.prank(alice);
        if (amt > limit) {
            vm.expectRevert();
            vault.borrow(tokenId, amt);
        } else {
            vault.borrow(tokenId, amt);
            (, uint256 debt) = vault.positions(tokenId);
            assertLe(debt, limit);
        }
    }
}

contract DemoCardTest is Test {
    AppraisalRegistry reg;
    GradedCard cards;
    TestUSD usd;
    CardLendingVault vault;
    address admin = address(0xA11CE);
    address attester = address(0xA77E5);
    address judge = address(0x10D6E);

    function setUp() public {
        vm.warp(1_790_000_000);
        reg = new AppraisalRegistry(admin, attester);
        cards = new GradedCard(admin);
        usd = new TestUSD(admin);
        vault = new CardLendingVault(admin, reg, cards, usd);
        vm.startPrank(admin);
        usd.mint(address(vault), 1_000_000e6);
        cards.mint(admin, "PSA", "109308847", "10", "Rayquaza VMAX 218");   // the real tokenized slab
        cards.addDemoCard("PSA", "109308847", "10", "Rayquaza VMAX 218");
        vm.stopPrank();
        vm.prank(attester);
        reg.publish("PSA", "109308847", 258651, 290000, 321349, 5000, 860, 20,
            AppraisalRegistry.Confidence.REAL, AppraisalRegistry.Risk.A, true);
    }

    function test_anyoneCanClaimDemoAndBorrow() public {
        vm.startPrank(judge);
        uint256 id = cards.claimDemo(0);
        assertTrue(cards.isDemo(id));
        assertEq(cards.ownerOf(id), judge);
        assertEq(cards.certKeyOf(id), reg.certKey("PSA", "109308847"));   // same certificate
        cards.approve(address(vault), id);
        vault.deposit(id);
        assertEq(vault.borrowLimit(id), 1_293_255_000);                  // $1,293.255
        vault.borrow(id, 100e6);
        assertEq(usd.balanceOf(judge), 100e6);
        usd.approve(address(vault), 100e6);
        vault.repay(id, 100e6);
        vault.withdraw(id);
        vm.stopPrank();
        assertEq(cards.ownerOf(id), judge);
    }

    function test_realTokenIsNotDemo() public view {
        assertFalse(cards.isDemo(1));
    }

    function test_demoCooldown() public {
        vm.startPrank(judge);
        cards.claimDemo(0);
        vm.expectRevert(abi.encodeWithSelector(GradedCard.DemoCooldown.selector, block.timestamp + 10 minutes));
        cards.claimDemo(0);
        vm.warp(block.timestamp + 10 minutes);
        cards.claimDemo(0);
        vm.stopPrank();
    }

    function test_badDemoIndex() public {
        vm.prank(judge);
        vm.expectRevert(GradedCard.BadDemoIndex.selector);
        cards.claimDemo(5);
    }

    function test_onlyOwnerAddsDemoCards() public {
        vm.prank(judge);
        vm.expectRevert();
        cards.addDemoCard("PSA", "1", "10", "x");
    }

    function test_demoCopyCannotStealRealPosition() public {
        // admin deposits the real slab and borrows; a judge's demo copy is a separate position
        vm.startPrank(admin);
        cards.approve(address(vault), 1);
        vault.deposit(1);
        vault.borrow(1, 500e6);
        vm.stopPrank();
        vm.startPrank(judge);
        uint256 id = cards.claimDemo(0);
        vm.expectRevert(CardLendingVault.NotOwner.selector);
        vault.borrow(1, 1e6);            // the real slab's loan belongs to admin
        vm.expectRevert(CardLendingVault.NotDeposited.selector);
        vault.borrow(id, 1e6);           // demo copy must be deposited first
        vm.stopPrank();
        (address o, uint256 d) = vault.positions(1);
        assertEq(o, admin); assertEq(d, 500e6);
    }
}
