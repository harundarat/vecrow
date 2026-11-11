//! Mock NEP-141 token used as a USDC stand-in in local and sandbox tests.
//!
//! It is the standard `near-contract-standards` fungible token (NEP-141 core,
//! NEP-145 storage management, NEP-148 metadata) plus an open `mint` method.
//! Like real USDC, an account must be registered with `storage_deposit` before
//! it can receive tokens, so tests can exercise transfers to unregistered
//! accounts. Never deploy it with real value.

use near_contract_standards::fungible_token::metadata::{
    FT_METADATA_SPEC, FungibleTokenMetadata, FungibleTokenMetadataProvider,
};
use near_contract_standards::fungible_token::{
    FungibleToken, FungibleTokenCore, FungibleTokenResolver,
};
use near_contract_standards::storage_management::{
    StorageBalance, StorageBalanceBounds, StorageManagement,
};
use near_sdk::json_types::U128;
use near_sdk::{AccountId, BorshStorageKey, NearToken, PanicOnDefault, PromiseOrValue, near};

#[derive(BorshStorageKey)]
#[near]
enum StorageKey {
    Accounts,
}

#[near(contract_state)]
#[derive(PanicOnDefault)]
pub struct MockFt {
    token: FungibleToken,
    symbol: String,
    decimals: u8,
}

#[near]
impl MockFt {
    #[init]
    pub fn new(symbol: String, decimals: u8) -> Self {
        Self {
            token: FungibleToken::new(StorageKey::Accounts),
            symbol,
            decimals,
        }
    }

    /// Mints `amount` to a registered account. Open to anyone: test token only.
    pub fn mint(&mut self, account_id: AccountId, amount: U128) {
        self.token.internal_deposit(&account_id, amount.0);
    }
}

#[near]
impl FungibleTokenCore for MockFt {
    #[payable]
    fn ft_transfer(&mut self, receiver_id: AccountId, amount: U128, memo: Option<String>) {
        self.token.ft_transfer(receiver_id, amount, memo)
    }

    #[payable]
    fn ft_transfer_call(
        &mut self,
        receiver_id: AccountId,
        amount: U128,
        memo: Option<String>,
        msg: String,
    ) -> PromiseOrValue<U128> {
        self.token.ft_transfer_call(receiver_id, amount, memo, msg)
    }

    fn ft_total_supply(&self) -> U128 {
        self.token.ft_total_supply()
    }

    fn ft_balance_of(&self, account_id: AccountId) -> U128 {
        self.token.ft_balance_of(account_id)
    }
}

#[near]
impl FungibleTokenResolver for MockFt {
    #[private]
    fn ft_resolve_transfer(
        &mut self,
        sender_id: AccountId,
        receiver_id: AccountId,
        amount: U128,
    ) -> U128 {
        let (used_amount, _burned_amount) =
            self.token
                .internal_ft_resolve_transfer(&sender_id, receiver_id, amount);
        used_amount.into()
    }
}

#[near]
impl StorageManagement for MockFt {
    #[payable]
    fn storage_deposit(
        &mut self,
        account_id: Option<AccountId>,
        registration_only: Option<bool>,
    ) -> StorageBalance {
        self.token.storage_deposit(account_id, registration_only)
    }

    #[payable]
    fn storage_withdraw(&mut self, amount: Option<NearToken>) -> StorageBalance {
        self.token.storage_withdraw(amount)
    }

    #[payable]
    fn storage_unregister(&mut self, force: Option<bool>) -> bool {
        self.token.internal_storage_unregister(force).is_some()
    }

    fn storage_balance_bounds(&self) -> StorageBalanceBounds {
        self.token.storage_balance_bounds()
    }

    fn storage_balance_of(&self, account_id: AccountId) -> Option<StorageBalance> {
        self.token.storage_balance_of(account_id)
    }
}

#[near]
impl FungibleTokenMetadataProvider for MockFt {
    fn ft_metadata(&self) -> FungibleTokenMetadata {
        FungibleTokenMetadata {
            spec: FT_METADATA_SPEC.to_string(),
            name: format!("Mock {}", self.symbol),
            symbol: self.symbol.clone(),
            icon: None,
            reference: None,
            reference_hash: None,
            decimals: self.decimals,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use near_sdk::test_utils::{VMContextBuilder, accounts};
    use near_sdk::testing_env;

    #[test]
    fn mint_requires_registration_and_updates_supply() {
        let mut context = VMContextBuilder::new();
        testing_env!(
            context
                .attached_deposit(NearToken::from_millinear(10))
                .build()
        );
        let mut ft = MockFt::new("USDC".to_string(), 6);
        ft.storage_deposit(Some(accounts(1)), Some(true));
        ft.mint(accounts(1), U128(1_000));
        assert_eq!(ft.ft_balance_of(accounts(1)).0, 1_000);
        assert_eq!(ft.ft_total_supply().0, 1_000);
        assert!(ft.storage_balance_of(accounts(2)).is_none());
    }

    #[test]
    #[should_panic(expected = "is not registered")]
    fn mint_to_unregistered_account_fails() {
        testing_env!(VMContextBuilder::new().build());
        let mut ft = MockFt::new("USDC".to_string(), 6);
        ft.mint(accounts(2), U128(1));
    }
}
