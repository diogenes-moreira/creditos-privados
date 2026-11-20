/** Contact data published in the Terms and the Privacy Policy (v1.0). */
export const contact = {
  email: "atencion@cuppogan.com",
  whatsappDisplay: "+1 (201) 350-6141",
  whatsappNumber: "12013506141",
  address: "Calle 37 No. 14-31, Bogotá D.C.",
};

export const whatsappLink = (text = "Hola, quiero activar mi beneficio CuppoGan") =>
  `https://wa.me/${contact.whatsappNumber}?text=${encodeURIComponent(text)}`;

/** CuppoGan brandbook v2.0, the same tokens the app uses. */
export const brand = {
  deepField: "#06673D",
  midGreen: "#267E37",
  growthGreen: "#398E33",
  graphite: "#2D3131",
  warmCream: "#F4F2EB",
};
