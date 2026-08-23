package usbgadget

import (
	"fmt"
	"os/exec"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

const (
	ncmFirewallTableName = "jetkvm"
	ncmFirewallChainName = "input_usb0"
)

// applyNcmFirewall isolates the target host from JetKVM's management plane.
// Replies belonging to connections initiated by JetKVM are allowed so the
// RDP bridge can dial the Windows RDP service over usb0. DHCP requests are
// also allowed. New TCP/UDP connections from the target are dropped.
func (u *UsbGadget) applyNcmFirewall() error {
	if out, err := exec.Command("modprobe", "nf_tables").CombinedOutput(); err != nil {
		return fmt.Errorf("modprobe nf_tables: %w: %s", err, out)
	}

	conn, err := nftables.New()
	if err != nil {
		return fmt.Errorf("open nftables conn: %w", err)
	}

	table := &nftables.Table{Name: ncmFirewallTableName, Family: nftables.TableFamilyINet}
	conn.DelTable(table)
	_ = conn.Flush()

	table = conn.AddTable(table)
	policy := nftables.ChainPolicyAccept
	chain := conn.AddChain(&nftables.Chain{
		Name:     ncmFirewallChainName,
		Table:    table,
		Hooknum:  nftables.ChainHookInput,
		Priority: nftables.ChainPriorityFilter,
		Type:     nftables.ChainTypeFilter,
		Policy:   &policy,
	})

	// usb0 + ct state established,related => accept.
	stateMask := expr.CtStateBitESTABLISHED | expr.CtStateBitRELATED
	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ifnameBytes(ncmInterfaceName)},
			&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           binaryutil.NativeEndian.PutUint32(stateMask),
				Xor:            binaryutil.NativeEndian.PutUint32(0),
			},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte{0, 0, 0, 0}},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})

	// Allow DHCP client requests to the embedded DHCP server on UDP/67.
	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ifnameBytes(ncmInterfaceName)},
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_UDP}},
			&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: 2, Len: 2},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: binaryutil.BigEndian.PutUint16(67)},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})

	// Drop all new TCP/UDP traffic arriving from the target host. ICMP/ICMPv6
	// remains available for NDP and diagnostics.
	for _, proto := range []byte{unix.IPPROTO_TCP, unix.IPPROTO_UDP} {
		conn.AddRule(&nftables.Rule{
			Table: table,
			Chain: chain,
			Exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ifnameBytes(ncmInterfaceName)},
				&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{proto}},
				&expr.Verdict{Kind: expr.VerdictDrop},
			},
		})
	}

	if err := conn.Flush(); err != nil {
		return fmt.Errorf("commit nftables ruleset: %w", err)
	}
	return nil
}

func (u *UsbGadget) removeNcmFirewall() {
	conn, err := nftables.New()
	if err != nil {
		u.log.Warn().Err(err).Msg("nftables open failed during teardown")
		return
	}
	conn.DelTable(&nftables.Table{Name: ncmFirewallTableName, Family: nftables.TableFamilyINet})
	if err := conn.Flush(); err != nil {
		u.log.Debug().Err(err).Msg("nftables flush during teardown")
	}
}

func ifnameBytes(name string) []byte {
	b := make([]byte, unix.IFNAMSIZ)
	copy(b, name)
	return b
}
