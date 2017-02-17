local p_bidcos = Proto("bidcos", "BidCos");

local f_typ = ProtoField.uint8("bidcos.typ", "Type", base.DEC, { [5] = "APP_RECV" })
local f_status = ProtoField.uint8("bidcos.status", "Status", base.DEC);
local f_info = ProtoField.uint8("bidcos.info", "Info", base.DEC);
local f_rssi = ProtoField.uint8("bidcos.rssi", "RSSI", base.DEC);
local f_mnr = ProtoField.uint8("bidcos.mnr", "Message Counter", base.DEC);
local f_flags = ProtoField.uint8("bidcos.flags", "Flags", base.DEC);
local cmds = {
	[0x00] = "Device Info",
	[0x01] = "Configuration",
	-- TODO: 0x02 has subtypes
	[0x03] = "AESreply",
	[0x04] = "AESkey",
	[0x10] = "Information",
	[0x11] = "SET",
	[0x12] = "HAVE_DATA",
	[0x3e] = "Switch",
	[0x3f] = "Timestamp",
	[0x40] = "Remote",
	[0x41] = "Sensor",
	[0x53] = "Water sensor",
	[0x54] = "Gas sensor",
	[0x58] = "Climate event",
	[0x5a] = "Thermal control",
	[0x5e] = "Power event",
	[0x5f] = "Power event",
	[0x70] = "Weather event",
	[0xca] = "Firmware",
	[0xcb] = "RF configuration",
}
local f_cmd = ProtoField.uint8("bidcos.cmd", "Cmd", base.DEC, cmds);
local f_src = ProtoField.uint24("bidcos.src", "Src", base.DEC);
local f_dst = ProtoField.uint24("bidcos.dst", "Dest", base.DEC);
local f_payload = ProtoField.new("bidcos.payload", "payload", ftypes.BYTES);

p_bidcos.fields = { f_typ, f_status, f_info, f_rssi, f_mnr, f_flags, f_cmd, f_src, f_dst, f_payload }

function p_bidcos.dissector(buf, pkt, tree)
	local subtree = tree:add(p_bidcos, buf(0))
	subtree:add(f_typ, buf(0,1))
	subtree:add(f_status, buf(1,1))
	subtree:add(f_info, buf(2,1))
	subtree:add(f_rssi, buf(3,1))
	subtree:add(f_mnr, buf(4,1))
	subtree:add(f_flags, buf(5,1))
	subtree:add(f_cmd, buf(6,1))
	subtree:add(f_src, buf(7,3))
	subtree:add(f_dst, buf(10,3))
	rest = buf:len()-13-2
	if rest > 0 then
		subtree:add(f_payload, buf(13, rest))
	end
end

local p_hm = Proto("homematic", "HM-UARTGW");

local f_frame = ProtoField.uint8("homematic.frame", "Frame delimiter", base.DEC, { [253] = "valid frame" })
local f_len = ProtoField.uint16("homematic.len", "Packet length", base.DEC)
local f_dest = ProtoField.uint8("homematic.dest", "Destination", base.DEC, { [1] = "APP" })
local f_devcnt = ProtoField.uint8("homematic.cnt", "Message Counter", base.DEC)

p_hm.fields = { f_frame, f_len, f_dest, f_devcnt }

function p_hm.dissector(buf, pkt, tree)
	local subtree = tree:add(p_hm, buf(0))
	subtree:add(f_frame, buf(0,1))
	subtree:add(f_len, buf(1,2))
	subtree:add(f_dest, buf(3,1))
	subtree:add(f_devcnt, buf(4,1))

	local dissector = Dissector.get("bidcos")
	if dissector ~= nil then
		dissector:call(buf(5):tvb(), pkt, tree)
	end
end

local wtap_encap_table = DissectorTable.get("wtap_encap")
local udp_encap_table = DissectorTable.get("udp.port")

wtap_encap_table:add(wtap.USER15, p_hm)
wtap_encap_table:add(wtap.USER12, p_hm)
udp_encap_table:add(6080, p_hm)
