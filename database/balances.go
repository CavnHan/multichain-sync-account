package database

import (
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum/go-ethereum/common"
)

type Balances struct {
	GUID         uuid.UUID      `gorm:"primaryKey" json:"guid"`
	Address      common.Address `json:"address" gorm:"serializer:bytes"`
	TokenAddress common.Address `json:"token_address" gorm:"serializer:bytes"`
	Balance      *big.Int       `gorm:"serializer:u256;column:balance" db:"balance" json:"Balance" form:"balance"`
	LockBalance  *big.Int       `gorm:"serializer:u256;column:lock_balance" db:"lock_balance" json:"LockBalance" form:"lock_balance"`
	Timestamp    uint64
}

type BalancesView interface {
	QueryWalletBalanceByTokenAndAddress(requestId string, address, tokenAddress common.Address) (*Balances, error)
	UnCollectionList(requestId string, amount *big.Int) ([]Balances, error)
	QueryHotWalletBalances(requestId string, amount *big.Int) ([]Balances, error)
	QueryBalancesByToAddress(requestId string, address *common.Address) (*Balances, error)
}

type BalancesDB interface {
	BalancesView

	UpdateOrCreate(string, []TokenBalance) error
	StoreBalances(string, []Balances) error
	UpdateBalances(string, []Balances, bool) error
}

type balancesDB struct {
	gorm *gorm.DB
}

func NewBalancesDB(db *gorm.DB) BalancesDB {
	return &balancesDB{gorm: db}
}

func (db *balancesDB) StoreBalances(requestId string, balanceList []Balances) error {
	result := db.gorm.Table("balances_"+requestId).CreateInBatches(&balanceList, len(balanceList))
	return result.Error
}

func (db *balancesDB) UpdateBalances(requestId string, balanceList []Balances, isCollection bool) error {
	for i := 0; i < len(balanceList); i++ {
		var balance = Balances{}
		result := db.gorm.Table("balances_" + requestId).Where(&Balances{Address: balanceList[i].Address, TokenAddress: balanceList[i].TokenAddress}).Take(&balance)
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return nil
			}
			return result.Error
		}
		if isCollection {
			balance.LockBalance = balance.Balance
			balance.Balance = big.NewInt(0)
		} else {
			balance.Balance = new(big.Int).Sub(balance.Balance, balanceList[i].LockBalance)
			balance.LockBalance = balanceList[i].LockBalance
		}
		err := db.gorm.Table("balances_" + requestId).Save(&balance).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (db *balancesDB) QueryBalancesByToAddress(requestId string, address *common.Address) (*Balances, error) {
	var balanceEntry Balances
	err := db.gorm.Table("balances_"+requestId).Where("address", strings.ToLower(address.String())).Take(&balanceEntry).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &balanceEntry, nil
}

func (db *balancesDB) QueryHotWalletBalances(requestId string, amount *big.Int) ([]Balances, error) {
	var balanceList []Balances
	err := db.gorm.Table("balances_"+requestId).Where("address_type = ? and balance >=?", 1, amount.Uint64()).Find(&balanceList).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return balanceList, nil
}

func (db *balancesDB) UnCollectionList(requestId string, amount *big.Int) ([]Balances, error) {
	var balanceList []Balances
	err := db.gorm.Table("balances_"+requestId).Where("balance >=?", amount.Uint64()).Find(&balanceList).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return balanceList, nil
}

func (db *balancesDB) QueryWalletBalanceByTokenAndAddress(requestId string, address, tokenAddress common.Address) (*Balances, error) {
	var balanceEntry Balances
	err := db.gorm.Table("balances_"+requestId).Where("address = ? and token_address = ?", strings.ToLower(address.String()), strings.ToLower(tokenAddress.String())).Take(&balanceEntry).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &balanceEntry, nil
}

func (db *balancesDB) UpdateOrCreate(requestId string, balanceList []TokenBalance) error {
	hotWalletBalances, err := db.QueryHotWalletBalances(requestId, big.NewInt(0))
	if err != nil {
		log.Error("query hot wallet balances err", "err", err)
		return err
	}
	for _, value := range balanceList {
		var userBalanceEntry Balances
		err := db.gorm.Table("balances"+requestId).Where("address = ? and token_address = ? and address_type = ?", strings.ToLower(value.Address.String()), strings.ToLower(value.TokenAddress.String()), 0).Take(&userBalanceEntry).Error
		if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
			balanceValue := &Balances{
				GUID:         uuid.New(),
				Address:      value.Address,
				TokenAddress: value.TokenAddress,
				Balance:      value.Balance,
				LockBalance:  value.LockBalance,
				Timestamp:    uint64(time.Now().Unix()),
			}
			errC := db.gorm.Table("balances" + requestId).Create(balanceValue).Error
			if errC != nil {
				log.Error("create token info fail", "err", errC)
				return errC
			}
			return nil
		} else if err == nil {
			log.Info("handle balance update", "TxType", value.TxType)
			if value.TxType == 0 { // 0:充值；1:提现；2:归集；3:热转冷；4:冷转热
				userBalanceEntry.Balance = new(big.Int).Add(userBalanceEntry.Balance, value.Balance)
				log.Info("Deposit balance update", "TxType", value.TxType, "balance", value.Balance, "afterBalance", userBalanceEntry.Balance)
				errU := db.gorm.Table("balances" + requestId).Save(&userBalanceEntry).Error
				if errU != nil {
					return errU
				}
			} else if value.TxType == 1 { // 提现
				for _, hotWallet := range hotWalletBalances {
					if hotWallet.Address == value.Address && hotWallet.TokenAddress == value.TokenAddress {
						hotWallet.LockBalance = big.NewInt(0)
						errU := db.gorm.Table("balances" + requestId).Save(&hotWallet).Error
						if errU != nil {
							return errU
						}
					}
				}
			} else if value.TxType == 2 { // 归集
				if len(hotWalletBalances) > 0 {
					for _, hotWallet := range hotWalletBalances {
						if hotWallet.Address == value.Address && hotWallet.TokenAddress == value.TokenAddress {
							userBalanceEntry.LockBalance = big.NewInt(0)
							errU := db.gorm.Table("balances" + requestId).Save(&userBalanceEntry).Error
							if errU != nil {
								return errU
							}
							hotWallet.Balance = new(big.Int).Add(hotWallet.Balance, value.Balance)
							errU = db.gorm.Table("balances" + requestId).Save(&hotWallet).Error
							if errU != nil {
								return errU
							}
						}
					}
				}
			} else if value.TxType == 3 {
				if len(hotWalletBalances) > 0 {
					for _, hotWallet := range hotWalletBalances {
						hotWallet.LockBalance = big.NewInt(0)
						err := db.gorm.Table("balances" + requestId).Save(&hotWallet).Error
						if err != nil {
							return err
						}
					}
				}
			} else if value.TxType == 4 {
				if len(hotWalletBalances) > 0 {
					for _, hotWallet := range hotWalletBalances {
						hotWallet.Balance = new(big.Int).Add(hotWallet.Balance, value.Balance)
						err := db.gorm.Table("balances" + requestId).Save(&hotWallet).Error
						if err != nil {
							return err
						}
					}
				}
			}
		} else {
			log.Error("update or create balances fail", "err", err)
			continue
		}
	}
	return nil
}