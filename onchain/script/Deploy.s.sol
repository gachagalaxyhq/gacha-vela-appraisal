// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Script, console2} from "forge-std/Script.sol";
import {AppraisalRegistry} from "../src/AppraisalRegistry.sol";
import {GradedCard} from "../src/GradedCard.sol";
import {TestUSD} from "../src/TestUSD.sol";
import {CardLendingVault} from "../src/CardLendingVault.sol";

/// Deploys the full Buildathon stack to Robinhood Chain testnet (chain 46630).
///   forge script script/Deploy.s.sol --rpc-url robinhood_testnet --broadcast
contract Deploy is Script {
    function run() external {
        uint256 pk = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(pk);
        address attester = vm.envOr("ATTESTER", deployer);

        vm.startBroadcast(pk);
        AppraisalRegistry registry = new AppraisalRegistry(deployer, attester);
        GradedCard cards = new GradedCard(deployer);
        TestUSD usd = new TestUSD(deployer);
        CardLendingVault vault = new CardLendingVault(deployer, registry, cards, usd);
        usd.mint(address(vault), 1_000_000e6); // lending liquidity for the demo
        vm.stopBroadcast();

        console2.log("chainId            ", block.chainid);
        console2.log("AppraisalRegistry  ", address(registry));
        console2.log("GradedCard         ", address(cards));
        console2.log("TestUSD            ", address(usd));
        console2.log("CardLendingVault   ", address(vault));

        string memory j = "deployment";
        vm.serializeUint(j, "chainId", block.chainid);
        vm.serializeAddress(j, "registry", address(registry));
        vm.serializeAddress(j, "cards", address(cards));
        vm.serializeAddress(j, "usd", address(usd));
        string memory out = vm.serializeAddress(j, "vault", address(vault));
        vm.writeJson(out, string.concat("deployments/", vm.toString(block.chainid), ".json"));
    }
}
